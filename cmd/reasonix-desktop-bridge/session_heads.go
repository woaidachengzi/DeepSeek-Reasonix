package main

import (
	"fmt"

	"reasonix/internal/agent"
	"reasonix/internal/desktopbridge"
)

const maxBridgeSessionHeads = 100

func (r *controllerRuntime) SessionHeads() ([]desktopbridge.SessionHeadView, error) {
	heads, err := r.controller.SessionHeads()
	if err != nil {
		return nil, err
	}
	live := make([]agent.SessionHead, 0, len(heads))
	for _, head := range heads {
		if !head.Retired {
			live = append(live, head)
		}
	}
	heads = live
	// Keep the original head and the newest versions visible in long sessions.
	if len(heads) > maxBridgeSessionHeads {
		first := heads[0]
		selected := -1
		for i, head := range heads {
			if head.Selected {
				selected = i
				break
			}
		}
		start := len(heads) - maxBridgeSessionHeads + 1
		limited := make([]agent.SessionHead, 0, maxBridgeSessionHeads)
		limited = append(limited, first)
		if selected > 0 && selected < start {
			limited = append(limited, heads[selected])
			start++
		}
		heads = append(limited, heads[start:]...)
	}
	views := make([]desktopbridge.SessionHeadView, 0, len(heads))
	for _, head := range heads {
		views = append(views, desktopbridge.SessionHeadView{
			ID: head.ID, Kind: head.Kind, Name: truncateRunes(head.Name, 120),
			ParentID: head.ParentHead, Preview: truncateRunes(head.Preview, 160),
			MessageCount: head.MessageCount, Selected: head.Selected,
		})
	}
	return views, nil
}

func (r *controllerRuntime) SwitchSessionHead(headID string) error {
	if !validRevertID(headID) {
		return fmt.Errorf("invalid session head ID")
	}
	return r.controller.SwitchSessionHead(headID)
}

func truncateRunes(value string, limit int) string {
	chars := []rune(value)
	if len(chars) <= limit {
		return value
	}
	return string(chars[:limit-1]) + "…"
}
