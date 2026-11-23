//go:build !js

package main

func installTouchScroll() func() { return func() {} }
