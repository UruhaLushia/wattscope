package pd

import "slices"

// Analysis records evidence available when this packet arrived. Codes are
// language-independent. Durations use host receive times, not CC wire timing.
type Analysis struct {
	Stage                string              `json:"stage,omitempty"`
	CapabilitiesSequence uint64              `json:"capabilities_sequence,omitempty"`
	SourceCapabilities   *SourceCapabilities `json:"source_capabilities,omitempty"`
	RequestSequence      uint64              `json:"request_sequence,omitempty"`
	AcceptSequence       uint64              `json:"accept_sequence,omitempty"`
	ReadySequence        uint64              `json:"ready_sequence,omitempty"`
	AcknowledgesSequence uint64              `json:"acknowledges_sequence,omitempty"`
	AcknowledgesType     string              `json:"acknowledges_type,omitempty"`
	Request              *Request            `json:"request,omitempty"`
	IntervalSeconds      *float64            `json:"request_interval_s,omitempty"`
	SameRequest          bool                `json:"same_request,omitempty"`
	ResponseSeconds      *float64            `json:"response_s,omitempty"`
	ReadySeconds         *float64            `json:"ready_s,omitempty"`
	Findings             []string            `json:"findings,omitempty"`
}

func cloneAnalysis(a *Analysis) *Analysis {
	copy := *a
	copy.Findings = slices.Clone(a.Findings)
	if a.SourceCapabilities != nil {
		caps := *a.SourceCapabilities
		copy.SourceCapabilities = &caps
	}
	if a.Request != nil {
		r := *a.Request
		copy.Request = &r
	}
	if a.IntervalSeconds != nil {
		v := *a.IntervalSeconds
		copy.IntervalSeconds = &v
	}
	if a.ResponseSeconds != nil {
		v := *a.ResponseSeconds
		copy.ResponseSeconds = &v
	}
	if a.ReadySeconds != nil {
		v := *a.ReadySeconds
		copy.ReadySeconds = &v
	}
	return &copy
}

type conversation struct {
	sources        []PDO
	sourceSequence uint64
	sourceCaps     *SourceCapabilities
	pending        *Analysis
	current        *Analysis
	requestAt      float64
	previousAt     float64
	previousRaw    uint32
	hasPrevious    bool
	last           map[string]Packet
}

// Analyzer owns bounded state for the five recognized SOP channels. Its owner
// serializes Observe. Reconnects, capture gaps and resets discard stale context.
type Analyzer struct {
	channels map[string]*conversation
	segment  uint64
	sequence uint64
	elapsed  float64
	seen     bool
}

func (a *Analyzer) Reset() { *a = Analyzer{} }

func (a *Analyzer) Observe(p *Packet) {
	p.Analysis = &Analysis{}
	if a.seen && (p.Segment != a.segment || p.Elapsed < a.elapsed) {
		a.Reset()
	}
	if a.seen && a.sequence != 0 && p.Sequence != 0 && p.Sequence != a.sequence+1 {
		a.Reset()
		p.Analysis.Findings = append(p.Analysis.Findings, "capture_gap")
	}
	a.segment, a.sequence, a.elapsed, a.seen = p.Segment, p.Sequence, p.Elapsed, true
	if !recognizedSOP(p.SOP) {
		return
	}
	if a.channels == nil {
		a.channels = make(map[string]*conversation)
	}
	c := a.channels[p.SOP]
	if c == nil {
		c = &conversation{last: make(map[string]Packet)}
		a.channels[p.SOP] = c
	}
	defer func() {
		if source := a.channels["SOP"]; p.Error == "" && source != nil && source.sourceCaps != nil {
			caps := *source.sourceCaps
			p.Analysis.SourceCapabilities = &caps
		}
	}()
	if p.Error != "" {
		p.Analysis.Findings = append(p.Analysis.Findings, "invalid_packet")
		// An unreadable capability update makes the older PDO list uncertain.
		if p.Type == "Source_Capabilities" {
			c.sources, c.sourceSequence, c.pending, c.current = nil, 0, nil, nil
			c.sourceCaps = nil
			delete(c.last, p.PowerRole)
		}
		return
	}
	if p.Type == "Soft_Reset" || p.Type == "PR_Swap" || p.Type == "Data_Reset" {
		*c = conversation{last: make(map[string]Packet)}
		switch p.PowerRole {
		case "Source", "Sink", "Port", "Cable":
			c.last[p.PowerRole] = Clone(*p)
		}
		return
	}
	if p.Type == "DR_Swap" || p.Type == "VCONN_Swap" {
		if c.pending != nil {
			c.current = nil
		}
		c.pending = nil
	}
	p.Analysis.CapabilitiesSequence = c.sourceSequence
	if p.GoodCRC() {
		other := map[string]string{"Source": "Sink", "Sink": "Source", "Port": "Cable", "Cable": "Port"}[p.PowerRole]
		if previous, ok := c.last[other]; ok && previous.MessageID == p.MessageID {
			p.Analysis = cloneAnalysis(previous.Analysis)
			p.Analysis.Findings = slices.DeleteFunc(p.Analysis.Findings, func(code string) bool { return code == "retransmission" })
			p.Analysis.AcknowledgesSequence, p.Analysis.AcknowledgesType = previous.Sequence, previous.Type
		}
		return
	}
	previous, seen := c.last[p.PowerRole]
	if seen && p.MessageID == previous.MessageID && p.Header == previous.Header && p.MessageHex == previous.MessageHex {
		if previous.Request != nil {
			r := *previous.Request
			p.Request = &r
		}
		p.Analysis = cloneAnalysis(previous.Analysis)
		if !slices.Contains(p.Analysis.Findings, "retransmission") {
			p.Analysis.Findings = append(p.Analysis.Findings, "retransmission")
		}
		c.last[p.PowerRole] = Clone(*p)
		return
	}
	if p.Type == "Source_Capabilities" {
		c.sources, c.sourceSequence, c.pending, c.current = nil, 0, nil, nil
		c.sourceCaps = p.AdvertisedSource()
		p.Analysis.CapabilitiesSequence = 0
		if p.SOP != "SOP" || p.PowerRole == "Source" {
			c.sources, c.sourceSequence = slices.Clone(p.PDOs), p.Sequence
			p.Analysis.CapabilitiesSequence = p.Sequence
		} else {
			p.Analysis.Findings = append(p.Analysis.Findings, "unexpected_role")
		}
	}
	if p.Request != nil && (p.SOP != "SOP" || p.PowerRole == "Sink") {
		r := DecodeRequest(p.Request.Raw, c.sources)
		if p.Type == "EPR_Request" {
			r = DecodeRequest(p.Request.Raw, nil)
		}
		p.Request = &r
	} else if p.Request != nil {
		p.Analysis.Findings = append(p.Analysis.Findings, "unexpected_role")
	}
	// Port/cable messages have different semantics from Source/Sink negotiation.
	if p.SOP == "SOP" {
		switch {
		case p.Request != nil && p.PowerRole == "Sink":
			r := *p.Request
			x := p.Analysis
			x.Stage, x.RequestSequence, x.Request = "requested", p.Sequence, &r
			if c.pending != nil {
				x.Findings = append(x.Findings, "incomplete_previous_request")
			}
			if c.hasPrevious {
				x.IntervalSeconds = receiveDuration(p.Elapsed, c.previousAt)
				x.SameRequest = c.previousRaw == r.Raw
			}
			c.previousAt, c.previousRaw, c.hasPrevious = p.Elapsed, r.Raw, true
			if p.Type == "EPR_Request" {
				x.CapabilitiesSequence = 0
				x.Findings = append(x.Findings, "epr_context_unavailable")
			} else if len(c.sources) == 0 {
				x.Findings = append(x.Findings, "source_capabilities_missing")
			} else if r.Position < 1 || r.Position > len(c.sources) {
				x.Findings = append(x.Findings, "pdo_position_out_of_range")
			} else if r.Matched {
				x.Findings = append(x.Findings, requestFindings(r, c.sources[r.Position-1])...)
			} else {
				x.Findings = append(x.Findings, "unsupported_supply")
			}
			if r.Mismatch {
				x.Findings = append(x.Findings, "capability_mismatch")
			}
			c.pending, c.requestAt = cloneAnalysis(x), p.Elapsed
		case p.PowerRole == "Source" && (p.Type == "Accept" || p.Type == "Reject" || p.Type == "Wait" || p.Type == "PS_RDY"):
			if c.pending != nil {
				x := cloneAnalysis(c.pending)
				p.Analysis = x
				switch p.Type {
				case "Accept":
					x.Stage, x.AcceptSequence = "accepted", p.Sequence
					x.ResponseSeconds = receiveDuration(p.Elapsed, c.requestAt)
					c.pending = cloneAnalysis(x)
				case "Reject", "Wait":
					x.Stage = "rejected"
					if p.Type == "Wait" {
						x.Stage = "wait"
					}
					x.ResponseSeconds = receiveDuration(p.Elapsed, c.requestAt)
					c.pending = nil
				case "PS_RDY":
					x.Stage, x.ReadySequence = "ready", p.Sequence
					x.ReadySeconds = receiveDuration(p.Elapsed, c.requestAt)
					if x.AcceptSequence == 0 {
						x.Findings = append(x.Findings, "accept_not_observed")
					}
					c.pending = nil
				}
			}
		}
	}
	if p.Analysis.Stage != "" {
		c.current = cloneAnalysis(p.Analysis)
	} else if c.current != nil && p.Type != "Source_Capabilities" && p.Type != "Accept" && p.Type != "Reject" && p.Type != "Wait" && p.Type != "PS_RDY" {
		findings := p.Analysis.Findings
		p.Analysis = cloneAnalysis(c.current)
		p.Analysis.Findings = append(p.Analysis.Findings, findings...)
	}
	// Stored snapshots never change when later packets advance the conversation.
	switch p.PowerRole {
	case "Source", "Sink", "Port", "Cable":
		c.last[p.PowerRole] = Clone(*p)
	}
}

func receiveDuration(now, before float64) *float64 {
	if now < before {
		return nil
	}
	d := now - before
	return &d
}

func requestFindings(r Request, source PDO) []string {
	var findings []string
	if r.VoltageV > 0 && (r.VoltageV < source.MinV-0.001 || r.VoltageV > source.MaxV+0.001) {
		findings = append(findings, "voltage_out_of_range")
	}
	if source.MaxA > 0 && (r.OperatingA > source.MaxA+0.001 || !r.Mismatch && r.MaximumA > source.MaxA+0.001) {
		findings = append(findings, "current_exceeds_source")
	}
	if source.MaxW > 0 && (r.OperatingW > source.MaxW+0.001 || !r.Mismatch && r.MaximumW > source.MaxW+0.001 || r.VoltageV*r.OperatingA > source.MaxW+0.001) {
		findings = append(findings, "power_exceeds_source")
	}
	if r.GiveBack && (r.MinimumA > r.OperatingA+0.001 || r.MinimumW > r.OperatingW+0.001) || !r.GiveBack && (r.MaximumA > 0 && r.MaximumA < r.OperatingA-0.001 || r.MaximumW > 0 && r.MaximumW < r.OperatingW-0.001) {
		findings = append(findings, "inconsistent_request_limits")
	}
	return findings
}
