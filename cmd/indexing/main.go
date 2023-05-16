package main

import (
	"indexing/internal/indexing"
	"log"

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

	startWindow(a)

	_, err := indexing.GetIndexInstance()

	if err != nil {
		log.Fatal(err)
	}

	a.Wait()
}

func startWindow(a *astilectron.Astilectron) {
	w, err := a.NewWindow("./page/home.html", &astilectron.WindowOptions{
		Center: astikit.BoolPtr(true),
		Height: astikit.IntPtr(600),
		Width:  astikit.IntPtr(1000),
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

		files := idx.Search(s)

		w.SendMessage(files)
		return nil
	})
}
