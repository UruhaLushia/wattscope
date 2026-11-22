package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/internal/recording"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type filePickerKind uint8

const (
	pickerOpenCapture filePickerKind = iota
	pickerSaveCopy
)

type filePickerEntry struct {
	Name      string
	Path      string
	Directory bool
}

type filePickerData struct {
	ID               uint64
	DirSeq           uint64
	Kind             filePickerKind
	Open             bool
	Loading          bool
	Saving           bool
	Title            string
	Path             string
	Current          string
	Error            string
	ShowHidden       bool
	CanBack          bool
	CanForward       bool
	Entries          []filePickerEntry
	Locations        []filePickerEntry
	Extensions       []string
	SourcePath       string
	OverwriteConfirm string
}

type filePicker struct {
	data     filePickerData
	ui       filePickerViewState
	request  uint64
	cancel   context.CancelFunc
	history  []string
	position int
	win      *mygo.Window
	closing  bool
}

var captureExtensions = []string{".jsonl", ".ndjson", ".json"}

func (d *dashboard) openCapturePicker(msgs locales.Messages) {
	dir, _ := d.recordDir()
	startDir := initialPickerDirectory(dir)
	d.openFilePicker(filePickerData{
		Kind:       pickerOpenCapture,
		Title:      msgs.GuiOpenCapture(),
		Extensions: slices.Clone(captureExtensions),
	}, startDir)
}

func (d *dashboard) openSaveCopyPicker(msgs locales.Messages, sourcePath string) {
	var extensions []string
	if ext := strings.ToLower(filepath.Ext(sourcePath)); ext != "" {
		if slices.Contains(captureExtensions, ext) {
			extensions = slices.Clone(captureExtensions)
		} else {
			extensions = []string{ext}
		}
	}
	downloads, _ := mygo.App.Path(mygo.PathDownloads)
	startDir := initialPickerDirectory(downloads, filepath.Dir(sourcePath))
	d.openFilePicker(filePickerData{
		Kind:       pickerSaveCopy,
		Title:      msgs.GuiSaveCopy(),
		Path:       filepath.Base(sourcePath),
		Extensions: extensions,
		SourcePath: sourcePath,
	}, startDir)
}

func initialPickerDirectory(candidates ...string) string {
	if downloads, err := mygo.App.Path(mygo.PathDownloads); err == nil && downloads != "" {
		candidates = append(candidates, downloads)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, home)
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return string(filepath.Separator)
}

func (d *dashboard) openFilePicker(initial filePickerData, startDir string) {
	if d.ctx.Err() != nil {
		return
	}
	p := &d.picker
	if p.win != nil {
		p.win.Focus()
		return
	}
	p.request++
	p.history, p.position = nil, -1
	initial.ID = p.request
	initial.Open = true
	p.data = initial

	if d.window != nil {
		palette, _ := d.frameTheme(nil)
		p.win = mygo.NewWindow(mygo.WindowOptions{
			Title:             initial.Title,
			Width:             760,
			Height:            560,
			MinWidth:          640,
			MinHeight:         480,
			UseContentSize:    true,
			TitleBarStyle:     mygo.TitleBarHidden,
			TitleBarHeight:    40,
			Parent:            d.window,
			Modal:             true,
			SkipTaskbar:       true,
			HideWindowButtons: true,
			DisableMinimize:   true,
			DisableMaximize:   true,
			DisableFullScreen: true,
			BackgroundColor:   kit.HexColor(palette.Surface),
			Content:           ui.View(d.pickerView),
		})
		win := p.win
		win.OnClosed(func() {
			p.win, p.closing = nil, false
			if p.data.Open {
				d.resetFilePicker()
			}
			if d.ctx.Err() == nil && !d.window.IsDestroyed() {
				d.window.Invalidate()
			}
		})
	}
	d.readPickerDirectory(startDir, true, -1)
}

func (d *dashboard) pickerContext() (context.Context, uint64) {
	p := &d.picker
	if p.cancel != nil {
		p.cancel()
	}
	p.request++
	ctx, cancel := context.WithCancel(d.ctx)
	p.cancel = cancel
	return ctx, p.request
}

func (d *dashboard) pickerDispatcher(ctx context.Context, token uint64) func(func()) {
	dispatch := d.dispatchTo(d.picker.win)
	return func(apply func()) {
		if ctx.Err() != nil {
			return
		}
		dispatch(func() {
			if ctx.Err() == nil && d.picker.request == token && d.picker.data.Open {
				d.picker.cancel()
				d.picker.cancel = nil
				apply()
			}
		})
	}
}

func (d *dashboard) closeFilePicker() {
	p := &d.picker
	if p.closing {
		return
	}
	d.resetFilePicker()
	if win := p.win; win != nil {
		p.closing = true
		// Input actions may run inside GtkGLArea's render callback; destroy after callback exits.
		win.Update(func() {
			if p.win == win {
				win.Destroy()
			}
		})
	}
}

func (d *dashboard) resetFilePicker() {
	p := &d.picker
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.request++
	p.data = filePickerData{}
	p.history = nil
	p.position = -1
	p.ui = filePickerViewState{selected: -1}
}

func (d *dashboard) browsePickerDirectory(path string) {
	if !d.picker.data.Open || d.picker.data.Saving {
		return
	}
	d.readPickerDirectory(resolvePickerRelative(path, d.picker.data.Current), false, -1)
}

func (d *dashboard) movePickerHistory(delta int) {
	p := &d.picker
	pos := p.position + delta
	if !p.data.Open || p.data.Saving || pos < 0 || pos >= len(p.history) {
		return
	}
	d.readPickerDirectory(p.history[pos], false, pos)
}

func (d *dashboard) readPickerDirectory(path string, initial bool, position int) {
	p := &d.picker
	ctx, token := d.pickerContext()
	dispatch := d.pickerDispatcher(ctx, token)
	p.data.Loading = true
	p.data.Error = ""
	p.data.OverwriteConfirm = ""
	locations := p.data.Locations
	extensions := slices.Clone(p.data.Extensions)
	recDir, _ := d.recordDir()
	msgs := locales.In(d.locale())

	d.work.Go(func() {
		current, entries, err := readDirectoryEntries(ctx, msgs, path, initial, extensions)
		if locations == nil && ctx.Err() == nil {
			locations = buildPickerLocations(ctx, msgs, recDir)
		}
		dispatch(func() {
			data := &d.picker.data
			data.Loading = false
			data.Locations = locations
			if err != nil {
				data.Error = err.Error()
				return
			}
			prevCurrent := data.Current
			data.DirSeq++
			data.Current = current
			data.Entries = entries
			if position >= 0 {
				p.position = position
			} else if p.position < 0 || p.history[p.position] != current {
				p.history = append(p.history[:p.position+1], current)
				p.position = len(p.history) - 1
			}
			data.CanBack = p.position > 0
			data.CanForward = p.position+1 < len(p.history)
			if data.Kind == pickerOpenCapture && current != prevCurrent && !initial {
				data.Path = ""
			}
		})
	})
}

func (d *dashboard) createPickerFolder(msgs locales.Messages, name string) {
	p := &d.picker
	if !p.data.Open || p.data.Saving || p.data.Loading || p.data.Current == "" {
		return
	}
	name = strings.TrimSpace(name)
	if !isValidSingleName(name) {
		p.data.Error = msgs.GuiInvalidName()
		return
	}
	target := filepath.Join(p.data.Current, name)
	ctx, token := d.pickerContext()
	dispatch := d.pickerDispatcher(ctx, token)
	p.data.Saving = true
	p.data.Error = ""
	p.data.OverwriteConfirm = ""
	d.work.Go(func() {
		err := ctx.Err()
		if err == nil {
			err = os.Mkdir(target, 0o755)
		}
		dispatch(func() {
			d.picker.data.Saving = false
			if err != nil {
				d.picker.data.Error = err.Error()
				return
			}
			d.browsePickerDirectory(target)
		})
	})
}

func (d *dashboard) confirmFilePicker(msgs locales.Messages) {
	p := &d.picker
	if !p.data.Open || p.data.Saving || p.data.Loading {
		return
	}
	input := strings.TrimSpace(p.data.Path)
	if input == "" {
		p.data.Error = msgs.GuiSelectFile()
		return
	}
	resolved, err := resolvePickerPath(resolvePickerRelative(input, p.data.Current))
	if err != nil {
		p.data.Error = err.Error()
		return
	}

	if p.data.Kind == pickerOpenCapture {
		info, statErr := os.Stat(resolved)
		if statErr != nil {
			p.data.Error = statErr.Error()
			return
		}
		if info.IsDir() {
			d.browsePickerDirectory(resolved)
			return
		}
		d.closeFilePicker()
		d.loadCapturePath(resolved)
		return
	}

	if strings.HasSuffix(input, "/") || strings.HasSuffix(input, `\`) {
		p.data.Error = msgs.GuiInvalidName()
		return
	}
	if sameFilePath(p.data.SourcePath, resolved) {
		p.data.Error = msgs.GuiSameFile()
		return
	}
	if info, statErr := os.Stat(resolved); statErr == nil {
		if info.IsDir() {
			p.data.Error = msgs.GuiNotAFile()
			return
		}
		if p.data.OverwriteConfirm != resolved {
			p.data.OverwriteConfirm = resolved
			p.data.Error = ""
			return
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		p.data.Error = statErr.Error()
		return
	}

	srcPath := p.data.SourcePath
	ctx, token := d.pickerContext()
	dispatch := d.pickerDispatcher(ctx, token)
	p.data.Saving = true
	p.data.Error = ""
	d.work.Go(func() {
		err := copyFile(ctx, srcPath, resolved)
		dispatch(func() {
			d.picker.data.Saving = false
			if err != nil {
				d.picker.data.Error = err.Error()
				return
			}
			d.closeFilePicker()
			d.notify(resolved, "success")
			if d.window != nil {
				d.window.Invalidate()
			}
		})
	})
}

func (d *dashboard) loadCapturePath(path string) {
	d.runJob(func(ctx context.Context) (func(), string, error) {
		apply, err := d.readCaptureApply(ctx, path)
		return apply, "", err
	})
}

func (d *dashboard) readCaptureApply(ctx context.Context, path string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	packets, total, err := recording.LoadCapture(contextReader{ctx, file}, packetLimit)
	if err != nil {
		return nil, err
	}
	return func() {
		d.packets = packetView{path: path, offline: &backend.PacketSnapshot{
			Packets: packets, Total: total, Evicted: total - uint64(len(packets)),
		}}
		d.goPage(pagePD)
	}, nil
}

func resolvePickerRelative(path, current string) string {
	path = strings.TrimSpace(path)
	if path != "" && !filepath.IsAbs(path) && path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		if current != "" {
			return filepath.Join(current, path)
		}
	}
	return path
}

func resolvePickerPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is empty")
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	return filepath.Abs(path)
}

func isValidSingleName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

func readDirectoryEntries(ctx context.Context, msgs locales.Messages, path string, parentFallback bool, extensions []string) (string, []filePickerEntry, error) {
	resolved, err := resolvePickerPath(path)
	if err != nil {
		return "", nil, err
	}
	if info, statErr := os.Stat(resolved); statErr == nil && !info.IsDir() {
		if parentFallback {
			resolved = filepath.Dir(resolved)
		} else {
			return "", nil, errors.New(msgs.GuiNotADirectory())
		}
	}
	var dirEntries []os.DirEntry
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		dirEntries, err = os.ReadDir(resolved)
		if !parentFallback || !os.IsNotExist(err) || filepath.Dir(resolved) == resolved {
			break
		}
		resolved = filepath.Dir(resolved)
	}
	if err != nil {
		return "", nil, err
	}
	entries := make([]filePickerEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		child := filepath.Join(resolved, de.Name())
		isDir := de.IsDir()
		if !isDir && de.Type()&os.ModeSymlink != 0 {
			info, statErr := os.Stat(child)
			isDir = statErr == nil && info.IsDir()
		}
		allowed := isDir
		if !isDir {
			if len(extensions) == 0 {
				allowed = true
			} else {
				lower := strings.ToLower(de.Name())
				for _, ext := range extensions {
					if strings.HasSuffix(lower, ext) {
						allowed = true
						break
					}
				}
			}
		}
		if allowed {
			entries = append(entries, filePickerEntry{Name: de.Name(), Path: child, Directory: isDir})
		}
	}
	slices.SortStableFunc(entries, func(a, b filePickerEntry) int {
		if a.Directory != b.Directory {
			if a.Directory {
				return -1
			}
			return 1
		}
		return compareFold(a.Name, b.Name)
	})
	return resolved, entries, nil
}

func buildPickerLocations(ctx context.Context, msgs locales.Messages, recordingsDir string) []filePickerEntry {
	var locations []filePickerEntry
	seen := make(map[string]bool, 8)
	addDir := func(name, path string) {
		if path == "" || seen[path] {
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return
		}
		seen[path] = true
		locations = append(locations, filePickerEntry{Name: name, Path: path, Directory: true})
	}

	if home, err := os.UserHomeDir(); err == nil {
		addDir(msgs.GuiHome(), home)
	}
	for _, item := range []struct {
		name string
		kind mygo.PathName
	}{
		{msgs.GuiDesktop(), mygo.PathDesktop},
		{msgs.GuiDownloads(), mygo.PathDownloads},
		{msgs.GuiDocuments(), mygo.PathDocuments},
	} {
		if ctx.Err() != nil {
			break
		}
		if path, err := mygo.App.Path(item.kind); err == nil {
			addDir(item.name, path)
		}
	}
	if recordingsDir != "" {
		addDir(msgs.GuiRecordings(), recordingsDir)
	}
	if runtime.GOOS == "windows" {
		for drive := 'A'; drive <= 'Z'; drive++ {
			if ctx.Err() != nil {
				break
			}
			addDir(fmt.Sprintf("%c:", drive), fmt.Sprintf(`%c:\`, drive))
		}
	} else {
		addDir(msgs.GuiFilesystem(), string(filepath.Separator))
	}
	return locations
}

func sameFilePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ia, ea := os.Stat(a)
	ib, eb := os.Stat(b)
	return ea == nil && eb == nil && os.SameFile(ia, ib)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func copyFile(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dstDir, ".wattscope-copy-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	defer tmp.Close()

	if _, err := io.Copy(tmp, contextReader{ctx, srcFile}); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(tmpPath, dst)
}

func compareFold(a, b string) int {
	for len(a) > 0 && len(b) > 0 {
		x, nx := utf8.DecodeRuneInString(a)
		y, ny := utf8.DecodeRuneInString(b)
		x, y = unicode.ToLower(x), unicode.ToLower(y)
		if x != y {
			return cmp.Compare(x, y)
		}
		a, b = a[nx:], b[ny:]
	}
	return cmp.Compare(len(a), len(b))
}
