package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

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

		files := idx.Search(s)

		w.SendMessage(files)
		return nil
	})
}
