package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"ruuvi-listener/internal/service"
	"ruuvi-listener/pkg/storage"
)

// Run starts the Fyne application. It blocks until the window is closed.
func Run(store *storage.Store, sender *service.Sender, fmi *service.FmiCollector) {
	a := fyneapp.New()
	w := a.NewWindow("Ruuvi Listener")

	statusLabel := widget.NewLabel("Scanning…")
	lastSentLabel := widget.NewLabel("")
	countdownLabel := widget.NewLabel("")
	weatherLabel := widget.NewLabel("FMI: fetching…")

	// tags is the snapshot used by the list; always updated on the Fyne thread.
	var tags []*storage.Tag
	tags = store.All()

	var list *widget.List

	refreshList := func() {
		tags = store.All()
		list.Refresh()
	}

	list = widget.NewList(
		func() int { return len(tags) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewButton("▲", nil),
				widget.NewButton("▼", nil),
				widget.NewCheck("", nil),
				widget.NewLabel("placeholder name that is long enough"),
				layout.NewSpacer(),
				widget.NewLabel("−00.0°C"),
				widget.NewLabel("000.0%"),
				widget.NewLabel("00m ago"),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(tags) {
				return
			}
			t := tags[id]
			row := obj.(*fyne.Container)

			upBtn := row.Objects[0].(*widget.Button)
			downBtn := row.Objects[1].(*widget.Button)
			upBtn.OnTapped = func() {
				_ = store.Move(t.MAC, -1)
				refreshList()
			}
			downBtn.OnTapped = func() {
				_ = store.Move(t.MAC, 1)
				refreshList()
			}

			check := row.Objects[2].(*widget.Check)
			check.OnChanged = nil // prevent SetChecked from firing the stale handler
			check.SetChecked(t.Selected)
			check.OnChanged = func(checked bool) {
				if err := store.SetSelected(t.MAC, checked); err != nil {
					fyne.Do(func() { dialog.ShowError(err, w) })
				}
			}
			row.Objects[3].(*widget.Label).SetText(t.DisplayName())
			row.Objects[5].(*widget.Label).SetText(fmt.Sprintf("%.1f°C", t.Temperature))
			row.Objects[6].(*widget.Label).SetText(fmt.Sprintf("%.1f%%", t.Humidity))
			row.Objects[7].(*widget.Label).SetText(timeAgo(t.LastSeen))
		},
	)

	// Clicking a row opens a rename dialog.
	list.OnSelected = func(id widget.ListItemID) {
		if id >= len(tags) {
			return
		}
		t := tags[id]

		entry := widget.NewEntry()
		entry.SetText(t.Name)
		entry.SetPlaceHolder("e.g. Living Room")

		content := container.NewVBox(
			widget.NewLabel(t.MAC),
			entry,
		)
		dialog.ShowCustomConfirm("Rename tag", "Save", "Cancel", content,
			func(save bool) {
				list.UnselectAll()
				if !save {
					return
				}
				if err := store.SetName(t.MAC, entry.Text); err != nil {
					dialog.ShowError(err, w)
				}
				fyne.Do(func() {
					tags = store.All()
					list.Refresh()
				})
			},
			w,
		)
	}

	sendBtn := widget.NewButton("Send to TRMNL", func() {
		selected := store.AllSelected()
		if len(selected) == 0 {
			dialog.ShowInformation("Nothing to send", "No tags are checked. Select at least one tag to send.", w)
			return
		}
		go func() {
			err := sender.Send(selected)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, w)
				}
			})
		}()
	})

	payloadBtn := widget.NewButton("Last Payload", func() {
		payload := sender.LastPayload()
		if payload == "" {
			dialog.ShowInformation("Last Payload", "Nothing has been sent yet.", w)
			return
		}
		entry := widget.NewMultiLineEntry()
		entry.SetText(payload)
		entry.Wrapping = fyne.TextWrapOff

		pw := a.NewWindow("Last Sent Payload")
		pw.SetContent(container.NewScroll(entry))
		pw.Resize(fyne.NewSize(700, 500))
		pw.Show()
	})

	// Wire store updates → list refresh on the Fyne thread.
	store.SetOnChange(func() {
		fyne.Do(func() {
			tags = store.All()
			list.Refresh()
		})
	})

	// Tick every 5 seconds to keep "last seen", "sent X ago", countdown, and weather label current.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			fyne.Do(func() {
				list.Refresh()
				if t := sender.LastSent(); !t.IsZero() {
					lastSentLabel.SetText("Sent " + timeAgo(t))
				}
				countdownLabel.SetText(countdown(sender.NextSendAt()))
				if fmi != nil {
					weatherLabel.SetText(fmi.Status())
				}
			})
		}
	}()

	bottomBar := container.NewHBox(
		statusLabel,
		weatherLabel,
		layout.NewSpacer(),
		lastSentLabel,
		countdownLabel,
		payloadBtn,
		sendBtn,
	)

	w.SetContent(container.NewBorder(nil, bottomBar, nil, nil, list))
	w.Resize(fyne.NewSize(700, 380))
	w.ShowAndRun()
}

// timeAgo returns a human-readable age string for t.
func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t).Round(time.Second)
	switch {
	case d < 5*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// countdown returns "Next in Xm Ys" until the given time.
func countdown(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Until(t).Round(time.Second)
	if d <= 0 {
		return "Next in…"
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("Next in %dm %ds", m, s)
	}
	return fmt.Sprintf("Next in %ds", s)
}
