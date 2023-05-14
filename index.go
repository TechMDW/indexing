package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sync"
	"time"
)

var newFileIndex int

func indexDirectory(dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	lim := make(chan struct{}, maxcurrent)
	wg := sync.WaitGroup{}
	for _, file := range files {
		lim <- struct{}{}
		wg.Add(1)
		go func(file os.DirEntry) {
			defer func() { <-lim }()
			defer wg.Done()

			if err := indexFile(dir, file); err != nil {
				history <- fmt.Sprintf("Error indexing file %s: %s", file.Name(), err)
				log.Printf("Error indexing file %s: %s", file.Name(), err)
			}
		}(file)
	}

	wg.Wait()

	return nil
}

func indexFile(dir string, file fs.DirEntry) error {
	var filePath string
	if dir != "/" {
		filePath = dir + "/" + file.Name()
	} else {
		filePath = dir + file.Name()
	}

	for _, f := range fileIndex {
		if f.Path == filePath {
			if checksum(filePath, f.Hash.MD5) {
				history <- fmt.Sprintf("File %s already indexed", filePath)
				fmt.Println("File", filePath, "already indexed")
				return nil
			}
		}
	}

	fileStats, err := file.Info()

	if err != nil {
		return err
	}

	if file.IsDir() {
		err = indexDirectory(filePath)
		if err != nil {
			return err
		}

		fileIndexMutex.Lock()
		defer fileIndexMutex.Unlock()
		fileIndex = append(fileIndex, FileInfo{
			Name:        file.Name(),
			Path:        filePath,
			Size:        fileStats.Size(),
			IsDir:       fileStats.IsDir(),
			ModTime:     fileStats.ModTime(),
			Permissions: fileStats.Mode().Perm(),
		})

		newFileIndex++

		return nil
	}

	// Full path to the file
	startTime := time.Now()
	hash, err := hashFile(filePath)
	fmt.Println("Hashing file", filePath, "took", time.Since(startTime))
	history <- fmt.Sprintf("Hashing file %s took %s", filePath, time.Since(startTime))

	if err != nil {
		log.Printf("Error hashing file %s: %s", filePath, err)
		return err
	}

	fileIndexMutex.Lock()
	defer fileIndexMutex.Unlock()
	fileIndex = append(fileIndex, FileInfo{
		Name:        file.Name(),
		Path:        filePath,
		Size:        fileStats.Size(),
		IsDir:       fileStats.IsDir(),
		ModTime:     fileStats.ModTime(),
		Permissions: fileStats.Mode().Perm(),
		Hash:        hash,
	})

	newFileIndex++

	return nil
}

func autoStore(dir string) {
	man := time.NewTicker(1 * time.Minute)
	scan := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-man.C:
			go func() {
				if newFileIndex <= 0 {
					return
				}

				fileIndexMutex.Lock()
				defer fileIndexMutex.Unlock()

				err := storeIndex(dir)
				if err != nil {
					log.Println(err)
				}

				newFileIndex = 0
			}()

		case <-scan.C:
			go func() {
				if newFileIndex < 50 {
					return
				}

				fileIndexMutex.Lock()
				defer fileIndexMutex.Unlock()

				err := storeIndex(dir)
				if err != nil {
					log.Println(err)
				}

				newFileIndex = 0
			}()
		}
	}
}

func storeIndex(dir string) error {
	file, err := os.OpenFile(fmt.Sprintf("%s.TechMDW_indexing.json", dir), os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return err
	}

	defer file.Close()

	data, err := json.Marshal(fileIndex)

	if err != nil {
		return err
	}

	_, err = file.Write(data)

	if err != nil {
		return err
	}

	return nil
}

func loadFileIndex(dir string) error {
	file, err := os.Open(fmt.Sprintf("%s.TechMDW_indexing.json", dir))

	if err != nil {
		return err
	}

	defer file.Close()

	err = json.NewDecoder(file).Decode(&fileIndex)

	if err != nil {
		return err
	}

	return nil
}
