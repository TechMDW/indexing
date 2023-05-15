package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sort"
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
			if f.IsDir {
				fileStats, err := file.Info()

				if err != nil {
					if os.IsNotExist(err) {
						err := removeIndexFile(filePath)

						if err != nil {
							return err
						}
					}
					return err
				}

				if fileStats.ModTime().After(f.ModTime) {
					history <- fmt.Sprintf("File %s has been modified", filePath)

					fileIndexMutex.Lock()
					defer fileIndexMutex.Unlock()
					f.ModTime = fileStats.ModTime()
					f.Permissions = fileStats.Mode().Perm()
					f.Size = fileStats.Size()

					newFileIndex++
				}

				return nil
			}

			if checksum(filePath, f.Hash.MD5) {
				history <- fmt.Sprintf("File %s already indexed", filePath)
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
	// startTime := time.Now()
	hash, err := hashFile(filePath)
	// fmt.Println("Hashing file", filePath, "took", time.Since(startTime))
	// history <- fmt.Sprintf("Hashing file %s took %s", filePath, time.Since(startTime))

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

func autoIndexHandler(dir string) {
	forcedScan := time.NewTicker(1 * time.Minute)
	scan := time.NewTicker(1 * time.Second)
	del := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-forcedScan.C:
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

		case <-del.C:
			go func() {
				checkForRemovedFiles(fileIndex)
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

func searchIndex(query string) []FileInfo {
	type scoredFile struct {
		file      FileInfo
		score     int
		fileScore FileScore
		dirScore  DirScore
	}
	var scoredFiles []scoredFile
	seen := make(map[string]bool)

	for _, file := range fileIndex {
		if seen[file.Path] {
			continue
		}

		var score int
		var fileScore FileScore
		var dirScore DirScore
		if file.IsDir {
			score, dirScore = scoreDir(file, query)
		} else if !file.IsDir {
			score, fileScore = scoreFile(file, query)
		}

		if score > 0 {
			scoredFiles = append(scoredFiles, scoredFile{file, score, fileScore, dirScore})
			seen[file.Path] = true
		}
	}

	sort.Slice(scoredFiles, func(i, j int) bool {
		return scoredFiles[i].score > scoredFiles[j].score
	})

	if len(scoredFiles) > 500 {
		scoredFiles = scoredFiles[:500]
	}

	var results []FileInfo
	for _, scoredFile := range scoredFiles {
		scoredFile.file.Score = scoredFile.score
		scoredFile.file.FileScore = scoredFile.fileScore
		scoredFile.file.DirScore = scoredFile.dirScore
		results = append(results, scoredFile.file)
	}

	return results
}

// Takes in a copy of the fileIndex and checks if the files still exist
//
// If they don't exist, it calls removeIndexFile to remove the file from the "live index"
func checkForRemovedFiles(files []FileInfo) {
	for _, file := range files {
		if _, err := os.Stat(file.Path); err != nil {
			if os.IsNotExist(err) {
				err := removeIndexFile(file.Path)

				if err != nil {
					fmt.Println(err)
					continue
				}

				history <- fmt.Sprintf("File %s has been removed", file.Path)
			}
		}
	}

	return
}

func removeIndexFile(path string) error {
	fileIndexMutex.Lock()
	defer fileIndexMutex.Unlock()
	for i, f := range fileIndex {
		if f.Path == path {
			fileIndex = append(fileIndex[:i], fileIndex[i+1:]...)
			break
		}
	}

	return nil
}

// Loop through the files in the directory and subdirectories and check if they are in the index
func checkForNewFiles(dir string) {
	startTime := time.Now()
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Println(err)
		return
	}

	for _, file := range files {
		filePath := fmt.Sprintf("%s/%s", dir, file.Name())

		if file.IsDir() {
			checkForNewFiles(fmt.Sprintf("%s/%s", dir, file.Name()))
		}

		// Check if the file is in the index
		var found bool
		for _, f := range fileIndex {
			if f.Path == filePath {
				found = true
				break
			}
		}

		if found {
			continue
		}

		err := indexFile(dir, file)

		if err != nil {
			log.Println(err)
			return
		}
	}

	fmt.Println("Checking for new files took", time.Since(startTime))
}
