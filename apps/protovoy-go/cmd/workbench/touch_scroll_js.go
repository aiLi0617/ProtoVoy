package main

import (
	"math"
	"syscall/js"
)

// Viva 0.3 routes wheel events to ScrollArea but treats a touch drag as a
// pointer press. Translate vertical canvas gestures into the same wheel path.
func installTouchScroll() func() {
	window := js.Global()
	document := window.Get("document")
	style := document.Call("createElement", "style")
	style.Set("textContent", "canvas { touch-action: pan-x pinch-zoom; }")
	document.Get("head").Call("appendChild", style)

	const dragThreshold = 8
	var surface js.Value
	var pointerID int
	var startX, startY, lastY float64
	var tracking, dragging bool

	callback := js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := args[0]
		if event.Get("pointerType").String() != "touch" || !event.Get("isTrusted").Bool() {
			return nil
		}
		id := event.Get("pointerId").Int()
		phase := event.Get("type").String()
		if phase == "pointerdown" {
			target := event.Get("target")
			if !event.Get("isPrimary").Bool() {
				tracking = false
				return nil
			}
			tracking = target.Get("tagName").String() == "CANVAS"
			dragging = false
			surface, pointerID = target, id
			startX, startY = event.Get("clientX").Float(), event.Get("clientY").Float()
			lastY = startY
			return nil
		}
		if !tracking || id != pointerID {
			return nil
		}
		if phase != "pointermove" {
			tracking = false
			return nil
		}
		x, y := event.Get("clientX").Float(), event.Get("clientY").Float()
		if !dragging {
			if dy := math.Abs(y - startY); dy < dragThreshold || dy <= math.Abs(x-startX) {
				return nil
			}
			dragging = true
			// Release Viva's pressed control so a swipe starting on a button
			// cannot activate it when the finger is lifted.
			surface.Call("dispatchEvent", window.Get("PointerEvent").New("pointercancel", map[string]any{
				"pointerId": id, "pointerType": "touch", "clientX": x, "clientY": y,
			}))
		}
		surface.Call("dispatchEvent", window.Get("WheelEvent").New("wheel", map[string]any{
			"clientX": startX, "clientY": startY, "deltaY": lastY - y, "cancelable": true,
		}))
		lastY = y
		event.Call("stopPropagation")
		return nil
	})

	phases := []string{"pointerdown", "pointermove", "pointerup", "pointercancel"}
	for _, phase := range phases {
		document.Call("addEventListener", phase, callback, true)
	}
	return func() {
		for _, phase := range phases {
			document.Call("removeEventListener", phase, callback, true)
		}
		callback.Release()
		style.Call("remove")
	}
}
