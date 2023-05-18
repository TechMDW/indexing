package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/TechMDW/indexing/internal/indexing"

	"github.com/asticode/go-astikit"
	"github.com/asticode/go-astilectron"

	hook "github.com/robotn/gohook"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	a, _ := astilectron.New(nil, astilectron.Options{
		AppName: "Indexer",
	})

	defer a.Close()

	// Start astilectron
	a.Start()

	startWindow(a)

	_, err := indexing.GetIndexInstance()

	if err != nil {
		log.Fatal(err)
	}

	a.Wait()

	go func() {
		// get any keypress
		bufio.NewReader(os.Stdin).ReadByte()
		fmt.Println("Exiting...")
	}()
}

func startHook(w *astilectron.Window) {
	hook.Register(hook.KeyDown, []string{"ctrl", "space"}, func(event hook.Event) {
		if w.IsShown() {
			w.Hide()
		} else {
			w.Show()
			go w.Focus()
		}

		startHook(w)
		hook.End()
	})

	start := hook.Start()
	<-hook.Process(start)
}

func startWindow(a *astilectron.Astilectron) {
	w, err := a.NewWindow("./page/home.html", &astilectron.WindowOptions{
		Center:      astikit.BoolPtr(true),
		Height:      astikit.IntPtr(600),
		Width:       astikit.IntPtr(1000),
		Frame:       astikit.BoolPtr(false),
		Transparent: astikit.BoolPtr(true),
	})

	if err != nil {
		log.Fatal(err)
	}

	w.Create()
	w.OpenDevTools()
	listenForInput(w)
	startHook(w)
}

func listenForInput(w *astilectron.Window) {
	idx, err := indexing.GetIndexInstance()
	if err != nil {
		log.Fatal(err)
	}

	w.OnMessage(func(m *astilectron.EventMessage) interface{} {
		// Unmarshal
		var s string
		m.Unmarshal(&s)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		files := idx.Search(ctx, s)

		w.SendMessage(files)
		return nil
	})
}
