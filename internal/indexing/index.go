package indexing

import (
	"encoding/json"
	"fmt"
	"indexing/internal/hash"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// singelton main struct for the indexing package
var (
	once   sync.Once
	idx    *Index
	ErrIdx error
)

var lim = make(chan struct{}, MaxGoRoutines)
var wg = sync.WaitGroup{}

func IndexFile(path string, file fs.DirEntry) (*File, error) {
	info, err := file.Info()
	if err != nil {
		return nil, err
	}

	var hashes hash.Hash
	if !file.IsDir() {
		f, err := os.Open(fmt.Sprintf("%s/%s", path, file.Name()))
		if err != nil {
			return nil, err
		}
		defer f.Close()

		hashes, err = hash.HashFile(f)
		if err != nil {
			return nil, err
		}
	}

	return &File{
		Name:      file.Name(),
		Extension: filepath.Ext(file.Name()),
		Path:      path,
		FullPath:  fmt.Sprintf("%s/%s", path, file.Name()),
		Size:      info.Size(),
		IsHidden:  file.Name()[0] == '.',
		IsDir:     file.IsDir(),
		ModTime:   info.ModTime(),
		Permissions: Permissions{
			Permission: info.Mode(),
		},
		Hash: hashes,
	}, nil
}

func IndexDirectory(path string, index *Index) error {
	files, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, file := range files {
		lim <- struct{}{} // acquire a slot
		wg.Add(1)
		go func(file fs.DirEntry) {
			defer wg.Done()
			defer func() { <-lim }() // make sure to release the slot

			indexedFile, err := IndexFile(path, file)
			if err != nil {
				log.Printf("Error indexing file: %v", err)
				return
			}

			index.StoreIndex(indexedFile.FullPath, *indexedFile)

			if file.IsDir() {
				go IndexDirectory(fmt.Sprintf("%s/%s", path, file.Name()), index)
			}
		}(file)
	}

	wg.Wait()

	return nil
}

func GetIndexInstance() (*Index, error) {
	once.Do(func() {
		idx = &Index{
			FilesMap:   make(map[string]File),
			FilesArray: []File{},
		}

		rootPath, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}

		// Get windows or linux
		oss := runtime.GOOS

		switch oss {
		case "windows":
			rootPath = strings.Split(rootPath, ":")[0] + ":/"
		case "linux":
			if rootPath == "/" {
				break
			}

			split := strings.Split(rootPath, "/")
			rootPath = fmt.Sprintf("/%s", split[1])
		default:
		}

		idx.rootPath = rootPath

		err = idx.LoadFileIndex(rootPath)

		if err != nil {
			// If the index file doesn't exist or EOF, create a new one
			if err == os.ErrNotExist || err == io.EOF {
				err := IndexDirectory(rootPath, idx)
				if err != nil {
					ErrIdx = err
					return
				}

				err = idx.StoreFileIndex(rootPath)
				if err != nil {
					ErrIdx = err
					return
				}
			}

			ErrIdx = err
			return
		}

		go idx.handler()
	})

	return idx, ErrIdx
}

func (i *Index) handler() {
	var autoStoreFunc func()
	autoStoreFunc = func() {
		if i.newFilesSinceStore != 0 {
			i.StoreFileIndex(i.rootPath)
		}
		time.AfterFunc(1*time.Minute, autoStoreFunc)
	}
	go autoStoreFunc()

	var storeFunc func()
	storeFunc = func() {
		if i.newFilesSinceStore >= 50 {
			i.StoreFileIndex(i.rootPath)
		}
		time.AfterFunc(5*time.Second, storeFunc)
	}
	go storeFunc()

	var newFilesFunc func()
	newFilesFunc = func() {
		i.FindNewFiles(i.rootPath)
		time.AfterFunc(1*time.Minute, newFilesFunc)
	}

	var removedFilesFunc func()
	removedFilesFunc = func() {
		i.CheckForRemovedFiles()
		time.AfterFunc(30*time.Second, removedFilesFunc)
	}

	delayNewFiles := time.NewTimer(1 * time.Minute)
	delayRemovedFiles := time.NewTimer(30 * time.Second)

	for {
		select {
		case <-delayNewFiles.C:
			go newFilesFunc()
		case <-delayRemovedFiles.C:
			go removedFilesFunc()
		}
	}
}

func (i *Index) Search(q string) []File {
	var results []File

	i.FilesArrayLock.RLock()
	defer i.FilesArrayLock.RUnlock()
	for _, file := range i.FilesArray {
		var scoreTotal int
		var score_data interface{}

		if file.IsDir {
			scoreTotal, score_data = ScoreDir(file, q)
		} else {
			scoreTotal, score_data = ScoreFile(file, q)
		}

		if scoreTotal > 0 {
			file.internal_metadata.score = scoreTotal
			file.internal_metadata.score_data = score_data
			results = append(results, file)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].internal_metadata.score > results[j].internal_metadata.score
	})

	if len(results) > MaxResults {
		return results[:MaxResults]
	}

	return results
}

func (i *Index) FindNewFiles(path string) {
	files, err := os.ReadDir(path)
	if err != nil {
		log.Println(err)
		return
	}

	for _, file := range files {
		filePath := fmt.Sprintf("%s/%s", path, file.Name())

		if file.IsDir() {
			i.FindNewFiles(fmt.Sprintf("%s/%s", path, file.Name()))
			continue
		}

		i.FilesMapLock.RLock()
		currFile, found := i.FilesMap[filePath]
		i.FilesMapLock.RUnlock()

		if found && checksum(filePath, currFile.Hash.MD5) {
			continue
		}

		indexedFile, err := IndexFile(path, file)
		if err != nil {
			log.Println(err)
			return
		}

		err = i.StoreIndex(filePath, *indexedFile)
		if err != nil {
			log.Println(err)
			return
		}
	}
}

func (i *Index) CheckForRemovedFiles() {
	i.FilesArrayLock.RLock()
	files := i.FilesArray
	i.FilesArrayLock.RUnlock()

	for _, file := range files {
		if _, err := os.Stat(file.FullPath); err != nil {
			if os.IsNotExist(err) {
				err := i.RemoveIndex(file.FullPath)

				if err != nil {
					log.Println(err)
					continue
				}

				log.Printf("File %s has been removed", file.FullPath)
			}
		}
	}
}

func (i *Index) StoreIndex(fullPath string, file File) error {
	i.FilesMapLock.Lock()
	i.FilesMap[fullPath] = file
	i.FilesMapLock.Unlock()

	i.FilesArrayLock.RLock()

	var found bool
	var index int
	for y := 0; y < len(i.FilesArray); y++ {
		if i.FilesArray[y].FullPath == fullPath {
			found = true
			index = y
			break
		}
	}

	i.FilesArrayLock.RUnlock()

	if found {
		i.FilesArrayLock.Lock()
		i.FilesArray[index] = file
		i.FilesArrayLock.Unlock()
	} else {
		i.FilesArrayLock.Lock()
		i.FilesArray = append(i.FilesArray, file)
		i.FilesArrayLock.Unlock()
	}

	return nil
}

func (i *Index) RemoveIndex(path string) error {
	i.FilesMapLock.Lock()
	delete(i.FilesMap, path)
	i.FilesMapLock.Unlock()

	i.FilesArrayLock.Lock()
	defer i.FilesArrayLock.Unlock()

	var found bool
	for index, file := range i.FilesArray {
		if file.FullPath == path {
			i.FilesArray = append(i.FilesArray[:index], i.FilesArray[index+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrFileNotFound
	}

	return nil
}

func (i *Index) GetIndex(path string) (File, error) {
	i.FilesMapLock.RLock()
	defer i.FilesMapLock.RUnlock()

	file, ok := i.FilesMap[path]
	if !ok {
		return File{}, ErrFileNotFound
	}

	return file, nil
}

func (i *Index) GetIndexArray() []File {
	i.FilesArrayLock.RLock()
	defer i.FilesArrayLock.RUnlock()

	return i.FilesArray
}

func (i *Index) GetIndexMap() map[string]File {
	i.FilesMapLock.RLock()
	defer i.FilesMapLock.RUnlock()

	return i.FilesMap
}

func (i *Index) LoadFileIndex(path string) error {
	i.FilesMapLock.Lock()
	defer i.FilesMapLock.Unlock()
	i.FilesArrayLock.Lock()
	defer i.FilesArrayLock.Unlock()

	path = path + IndexFileName

	file, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&i.FilesMap); err != nil {
		return err
	}

	i.FilesArray = nil
	for _, file := range i.FilesMap {
		i.FilesArray = append(i.FilesArray, file)
	}

	return nil
}

func (i *Index) StoreFileIndex(path string) error {
	i.FilesMapLock.RLock()
	defer i.FilesMapLock.RUnlock()

	path = path + IndexFileName

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(i.FilesMap); err != nil {
		return err
	}

	i.lastStore = time.Now()
	i.newFilesSinceStore = 0

	return nil
}