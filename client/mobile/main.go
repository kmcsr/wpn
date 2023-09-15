
package main

import (
	goapp "golang.org/x/mobile/app"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
)

func main() {
	goapp.Main(func(app goapp.App) {
		for e := range app.Events() {
			switch e := app.Filter(e).(type) {
			case lifecycle.Event:
				_ = e
			case paint.Event:
				// Call OpenGL here.
				app.Publish()
			}
		}
	})
}
