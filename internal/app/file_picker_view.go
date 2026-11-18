package app

import (
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

type filePickerViewState struct {
	id             uint64
	dirSeq         uint64
	list           ui.ListState
	visible        []int
	creating       bool
	newFolder      string
	address        string
	filter         string
	selected       int
	addressHandle  ui.Handle
	editingAddress bool
	breadcrumbPath string
	filterSource   *filePickerEntry
	filterCount    int
	filterQuery    string
	filterHidden   bool
}

func (d *dashboard) pickerView(ctx *ui.Context) {
	ui.Overlay(ctx, func() {})
	d.drainUIQueue()
	_, palette, msgs := d.frameTheme(ctx)
	d.picker.ui.draw(ctx, palette, msgs, &d.picker.data, filePickerActions{
		Navigate: d.browsePickerDirectory,
		Create:   func(name string) { d.createPickerFolder(msgs, name) },
		Confirm:  func() { d.confirmFilePicker(msgs) },
		Cancel:   d.closeFilePicker,
		Back:     func() { d.movePickerHistory(-1) },
		Forward:  func() { d.movePickerHistory(1) },
	})
}

type filePickerActions struct {
	Navigate func(string)
	Create   func(string)
	Confirm  func()
	Cancel   func()
	Back     func()
	Forward  func()
}

func (v *filePickerViewState) draw(ctx *ui.Context, p kit.Palette, msgs locales.Messages, data *filePickerData, actions filePickerActions) {
	if v.id != data.ID {
		*v = filePickerViewState{id: data.ID, address: data.Current, selected: -1}
	}
	if v.dirSeq != data.DirSeq {
		v.dirSeq = data.DirSeq
		v.list = ui.ListState{}
		v.creating = false
		v.newFolder = ""
		v.address = data.Current
		v.filter = ""
		v.selected = -1
		v.editingAddress = false
		v.filterSource = nil
		v.filterCount = -1
	}
	if !data.Open {
		return
	}

	ctx.OnShortcut(0, ui.KeyEscape, func() {
		switch {
		case v.editingAddress:
			v.editingAddress = false
		case v.creating:
			v.creating = false
		default:
			actions.Cancel()
		}
	})
	ctx.OnShortcut(ui.Alt, ui.KeyLeft, actions.Back)
	ctx.OnShortcut(ui.Alt, ui.KeyRight, actions.Forward)
	ctx.OnShortcut(ui.Alt, ui.KeyUp, func() {
		if !data.Saving && data.Current != "" {
			actions.Navigate(filepath.Dir(data.Current))
		}
	})
	ctx.OnShortcut(ui.Cmd, ui.KeyL, func() { v.editAddress(ctx, data) })

	ui.Column(ctx).Fill().Background(p.Surface).Children(func() {
		bar := ctx.TitleBar()
		ui.Row(ctx).FillWidth().Height(max(40, bar.Height)).Shrink(0).
			Padding(0, bar.Right+12, 0, bar.Left+16).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Text(ctx, data.Title).FontSize(13).FontWeight(700).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			windowControlButton(ctx.Key("picker-close"), p, msgs.GuiClose(), kit.IconClose, true, actions.Cancel)
		})

		ui.Column(ctx).Grow(1).FillWidth().MinHeight(0).Padding(8, 16, 16, 16).Gap(12).Children(func() {
			v.navigation(ctx, p, msgs, data, actions)
			v.browser(ctx, p, msgs, data, actions)
			v.filenameRow(ctx, p, msgs, data, actions)
			if data.Error != "" {
				ui.Text(ctx, data.Error).FontSize(12).TextColor(p.Red).Wrap()
			} else if data.OverwriteConfirm != "" {
				ui.Text(ctx, msgs.GuiFileExists()).FontSize(12).TextColor(p.Warning).Wrap()
			}
			v.footer(ctx, p, msgs, data, actions)
		})
	})
}

func (v *filePickerViewState) navigation(ctx *ui.Context, p kit.Palette, msgs locales.Messages, data *filePickerData, actions filePickerActions) {
	ui.Row(ctx).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
		kit.IconActionButton(ctx.Key("picker-back"), p, kit.IconArrowLeft, msgs.GuiBack(), 34, false, actions.Back).
			Disabled(data.Saving || !data.CanBack)
		kit.IconActionButton(ctx.Key("picker-forward"), p, kit.IconArrowRight, msgs.GuiForward(), 34, false, actions.Forward).
			Disabled(data.Saving || !data.CanForward)
		kit.IconActionButton(ctx.Key("picker-up"), p, kit.IconArrowUp, msgs.GuiUp(), 34, false, func() {
			actions.Navigate(filepath.Dir(data.Current))
		}).Disabled(data.Saving || data.Current == "" || filepath.Dir(data.Current) == data.Current)

		if v.editingAddress || data.Current == "" {
			ui.TextInput(ctx.Key("picker-address"), &v.address).Bind(&v.addressHandle).
				Height(34).Radius(kit.RadiusS).Grow(1).MinWidth(0).
				Disabled(data.Saving).
				OnSubmit(func() { actions.Navigate(v.address) })
			kit.IconActionButton(ctx.Key("picker-go"), p, kit.IconArrowRight, msgs.GuiGo(), 34, false, func() {
				actions.Navigate(v.address)
			}).Disabled(data.Saving || strings.TrimSpace(v.address) == "")
		} else {
			v.breadcrumbs(ctx, p, data, actions)
			kit.IconActionButton(ctx.Key("picker-edit-path"), p, kit.IconEdit, msgs.GuiEditPath(), 34, false, func() {
				v.editAddress(ctx, data)
			}).Disabled(data.Saving)
		}
	})
}

func (v *filePickerViewState) editAddress(ctx *ui.Context, data *filePickerData) {
	if data.Saving {
		return
	}
	v.address, v.editingAddress = data.Current, true
	v.addressHandle.Focus(ctx)
}

func (v *filePickerViewState) breadcrumbs(ctx *ui.Context, p kit.Palette, data *filePickerData, actions filePickerActions) {
	ui.ScrollHorizontal(ctx).Scrollbars(false).Grow(1).MinWidth(0).Height(34).
		Radius(kit.RadiusM).Background(p.Inset).Border(1, p.LineSoft).Children(func() {
		ui.Row(ctx).Shrink(0).FillHeight().Padding(0, 6).Gap(2).AlignItems(ui.Center).Children(func() {
			root := filepath.VolumeName(data.Current) + string(filepath.Separator)
			parts := []filePickerEntry{{Name: root, Path: root}}
			relative := strings.TrimPrefix(data.Current, root)
			curPath := root
			for segment := range strings.SplitSeq(relative, string(filepath.Separator)) {
				if segment == "" {
					continue
				}
				curPath = filepath.Join(curPath, segment)
				parts = append(parts, filePickerEntry{Name: segment, Path: curPath})
			}
			for i, part := range parts {
				if i > 0 {
					ui.Icon(ctx, kit.IconChevronR).FontSize(12).TextColor(p.Muted).Shrink(0)
				}
				item := kit.ActionButton(ctx.Key(part.Path), p, kit.VariantGhost, nil, part.Name, false, func() {
					actions.Navigate(part.Path)
				}).Height(26).Padding(0, 8).Disabled(data.Saving)
				if i == len(parts)-1 && v.breadcrumbPath != data.Current {
					item.ScrollIntoView()
					v.breadcrumbPath = data.Current
				}
			}
		})
	})
}

func (v *filePickerViewState) browser(ctx *ui.Context, p kit.Palette, msgs locales.Messages, data *filePickerData, actions filePickerActions) {
	ui.Row(ctx).FillWidth().Grow(1).MinHeight(160).Radius(kit.RadiusL).Border(1, p.LineSoft).Clip().Children(func() {
		ui.Scroll(ctx.Key("picker-locations")).Scrollbars(false).Width(156).Shrink(0).FillHeight().Background(p.Inset).Children(func() {
			ui.Column(ctx).FillWidth().Shrink(0).Padding(8).Gap(2).Children(func() {
				ui.Text(ctx, msgs.GuiLocations()).FontSize(11).FontWeight(700).TextColor(p.Muted).Padding(6, 10)
				for _, location := range data.Locations {
					active := data.Current == location.Path
					row := ui.ButtonBase(ctx.Key(location.Path)).FillWidth().Height(32).Padding(0, 10).Gap(8).
						Radius(kit.RadiusS).AlignItems(ui.Center).Justify(ui.Start).Label(location.Name).
						Disabled(data.Saving).OnClick(func() { actions.Navigate(location.Path) })
					bg := ui.Transparent
					switch {
					case active:
						bg = p.AccentSoft
					case row.Pressed():
						bg = p.Press
					case row.Hovered() || row.FocusVisible():
						bg = p.Hover
					}
					kit.ControlMotion(row).Background(row.AnimateColor("loc-bg", bg, kit.MotionFast)).Children(func() {
						icon := kit.IconFolder
						if location.Name == msgs.GuiHome() {
							icon = kit.IconHome
						}
						color, ink, weight := p.Muted, p.Text, 500
						if active {
							color, ink, weight = p.AccentText, p.AccentText, 650
						}
						ui.Icon(ctx, icon).FontSize(15).TextColor(color).Shrink(0)
						ui.Text(ctx, location.Name).FontSize(13).FontWeight(weight).TextColor(ink).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
					})
				}
			})
		})

		ui.Box(ctx).Width(1).FillHeight().Background(p.LineSoft).Shrink(0)

		ui.Column(ctx).Grow(1).FillHeight().MinWidth(0).Padding(10).Gap(8).Children(func() {
			ui.Row(ctx).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				ui.TextInput(ctx.Key("picker-filter"), &v.filter).Height(32).Radius(kit.RadiusS).
					Grow(1).MinWidth(0).Placeholder(msgs.GuiFilterFiles()).Label(msgs.GuiFilterFiles()).
					Disabled(data.Saving).
					OnChange(func() { v.resetSelection() })
				ui.Checkbox(ctx.Key("picker-hidden"), &data.ShowHidden, msgs.GuiShowHidden()).
					Shrink(0).Disabled(data.Saving).
					OnChange(func() { v.resetSelection() })
			})

			if data.Loading {
				ui.Box(ctx).Grow(1).FillWidth().Center().Children(func() {
					ui.Spinner(ctx).Size(18, 18)
				})
				return
			}

			v.filterEntries(data)
			if len(v.visible) == 0 {
				label := msgs.GuiEmptyFolder()
				if v.filter != "" {
					label = msgs.GuiNoMatchFiles()
				}
				kit.EmptyState(ctx, p, kit.IconFolder, label, "", nil)
				return
			}

			v.list.Selected = &v.selected
			v.list.Label = func(i int) string { return data.Entries[v.visible[i]].Name }
			ui.List(ctx.Key(data.Current), &v.list, len(v.visible)).Grow(1).FillWidth().Padding(0, 4, 0, 0).
				Disabled(data.Saving).
				OnChange(func() {
					data.Error = ""
					data.OverwriteConfirm = ""
					if entry, ok := v.selectedEntry(data); ok && !entry.Directory {
						data.Path = entry.Name
					} else if data.Kind == pickerOpenCapture {
						data.Path = ""
					}
				}).
				OnSubmit(func() {
					if entry, ok := v.selectedEntry(data); ok {
						if entry.Directory {
							actions.Navigate(entry.Path)
						} else {
							data.Path = entry.Name
							data.Error = ""
							data.OverwriteConfirm = ""
							actions.Confirm()
						}
					}
				}).
				ItemKey(func(i int) any { return data.Entries[v.visible[i]].Path }).
				Scrollbars(false).
				Rows(func(row ui.ListRow) {
					entry := data.Entries[v.visible[row.Index]]
					ui.Row(row.Context).FillWidth().Height(34).Padding(0, 10).Gap(10).Radius(kit.RadiusS).
						AlignItems(ui.Center).Children(func() {
						icon, color, weight := kit.IconFileText, p.Muted, 450
						if entry.Directory {
							icon, color, weight = kit.IconFolder, p.AccentText, 550
						}
						if row.Selected() {
							color = p.AccentInk
							weight = 650
						}
						ui.Icon(row.Context, icon).FontSize(15).TextColor(color).Shrink(0)
						ui.Text(row.Context, entry.Name).FontSize(13).FontWeight(weight).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
					})
				})

			ui.Text(ctx, msgs.GuiItemsCount(len(v.visible))).FontSize(11).TextColor(p.Muted).Padding(0, 10)
		})
	})
}

func (v *filePickerViewState) filenameRow(ctx *ui.Context, p kit.Palette, msgs locales.Messages, data *filePickerData, actions filePickerActions) {
	ui.Row(ctx).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(ctx, msgs.GuiFilename()).FontSize(12).FontWeight(600).TextColor(p.Muted).MinWidth(56).Shrink(0)
		ui.TextInput(ctx.Key("picker-filename"), &data.Path).Height(34).Radius(kit.RadiusS).
			Grow(1).MinWidth(0).Label(msgs.GuiFilename()).
			Disabled(data.Loading || data.Saving).
			OnChange(func() {
				v.selected = -1
				data.Error = ""
				data.OverwriteConfirm = ""
			}).
			OnSubmit(actions.Confirm)
	})
}

func (v *filePickerViewState) footer(ctx *ui.Context, p kit.Palette, msgs locales.Messages, data *filePickerData, actions filePickerActions) {
	ui.Row(ctx).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if data.Kind == pickerSaveCopy {
			if v.creating {
				ui.TextInput(ctx.Key("picker-new-folder"), &v.newFolder).Width(176).Height(34).Radius(kit.RadiusS).
					Placeholder(msgs.GuiFolderName()).Label(msgs.GuiFolderName()).
					Disabled(data.Loading || data.Saving).
					OnSubmit(func() { actions.Create(v.newFolder) })
				kit.ActionButton(ctx.Key("picker-create-folder"), p, kit.VariantSecondary, nil, msgs.GuiCreate(), false, func() {
					actions.Create(v.newFolder)
				}).Disabled(data.Loading || data.Saving || strings.TrimSpace(v.newFolder) == "")
				kit.IconActionButton(ctx.Key("picker-cancel-folder"), p, kit.IconClose, msgs.GuiCancel(), 34, false, func() {
					v.creating = false
					data.Error = ""
				})
			} else {
				kit.ActionButton(ctx.Key("picker-new-folder-btn"), p, kit.VariantSecondary, kit.IconFolder, msgs.GuiNewFolder(), false, func() {
					v.creating = true
					v.newFolder = ""
					data.Error = ""
				}).Disabled(data.Loading || data.Saving || data.Current == "")
			}
		}

		ui.Spacer(ctx).MinWidth(0)

		kit.ActionButton(ctx.Key("picker-cancel"), p, kit.VariantSecondary, nil, msgs.GuiCancel(), false, actions.Cancel)

		confirmLabel, variant := msgs.GuiOpen(), kit.VariantPrimary
		if data.Kind == pickerSaveCopy {
			confirmLabel = msgs.GuiSave()
			if data.OverwriteConfirm != "" {
				confirmLabel = msgs.GuiOverwrite()
				variant = kit.VariantDanger
			}
		}
		kit.ActionButton(ctx.Key("picker-confirm"), p, variant, nil, confirmLabel, false, actions.Confirm).
			Disabled(data.Loading || data.Saving || strings.TrimSpace(data.Path) == "")
	})
}

func (v *filePickerViewState) filterEntries(data *filePickerData) {
	var source *filePickerEntry
	if len(data.Entries) > 0 {
		source = &data.Entries[0]
	}
	if source == v.filterSource && len(data.Entries) == v.filterCount && v.filter == v.filterQuery && data.ShowHidden == v.filterHidden {
		return
	}
	v.filterSource = source
	v.filterCount = len(data.Entries)
	v.filterQuery = v.filter
	v.filterHidden = data.ShowHidden

	query := strings.ToLower(strings.TrimSpace(v.filter))
	v.visible = v.visible[:0]
	for i, entry := range data.Entries {
		if !data.ShowHidden && strings.HasPrefix(entry.Name, ".") {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(entry.Name), query) {
			v.visible = append(v.visible, i)
		}
	}
}

func (v *filePickerViewState) selectedEntry(data *filePickerData) (filePickerEntry, bool) {
	if v.selected < 0 || v.selected >= len(v.visible) {
		return filePickerEntry{}, false
	}
	return data.Entries[v.visible[v.selected]], true
}

func (v *filePickerViewState) resetSelection() {
	v.list = ui.ListState{}
	v.selected = -1
}
