package main

import (
	"context"
	"fmt"
	"log"
	"runtime"
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

	go func() {
		for {
			PrintMemUsage()
			time.Sleep(5 * time.Second)
		}
	}()

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

func PrintMemUsage() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	fmt.Println("------Memory Usage------")
	fmt.Printf("Alloc = %v\n", ByteSize(mem.Alloc))
	fmt.Printf("TotalAlloc = %v\n", ByteSize(mem.TotalAlloc))
	fmt.Printf("Sys = %v\n", ByteSize(mem.Sys))
	fmt.Printf("NumGC = %v\n", mem.NumGC)
	fmt.Println("------------------------")
}

func ByteSize(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB",
		float64(bytes)/float64(div), "KMGTPE"[exp])
}
