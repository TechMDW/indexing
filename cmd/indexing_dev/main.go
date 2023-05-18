package main

import (
	"context"
	"log"
	"time"

	"github.com/TechMDW/indexing/internal/indexing"

	"github.com/asticode/go-astikit"
	"github.com/asticode/go-astilectron"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	a, _ := astilectron.New(nil, astilectron.Options{
		AppName: "Indexer",
	})

	defer a.Close()

	// Start astilectron
	a.Start()

	go startWindow(a)

	_, err := indexing.GetIndexInstance()

	if err != nil {
		log.Fatal(err)
	}

	a.Wait()
}

func startWindow(a *astilectron.Astilectron) {
	w, err := a.NewWindow("./page/home.html", &astilectron.WindowOptions{
		Center:      astikit.BoolPtr(true),
		MinHeight:   astikit.IntPtr(80),
		Width:       astikit.IntPtr(720),
		Frame:       astikit.BoolPtr(false),
		Transparent: astikit.BoolPtr(true),
	})

	if err != nil {
		log.Fatal(err)
	}

	w.Create()
	w.OpenDevTools()
	listenForInput(w)
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
