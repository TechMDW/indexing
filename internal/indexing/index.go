package indexing

import (
	"encoding/json"
	"errors"
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

	"github.com/pierrec/lz4/v4"
)

// singelton main struct for the indexing package
var (
	once   sync.Once
	idx    *Index
	ErrIdx error
)

var lim = make(chan struct{}, MaxGoRoutines)

func IndexFile(path string, file fs.DirEntry) (*File, error) {
	info, err := file.Info()
	if err != nil {
		return nil, err
	}

	var hashes hash.Hash
	var Error error = nil
	if !file.IsDir() {
		f, err := os.Open(fmt.Sprintf("%s/%s", path, file.Name()))
		if err != nil {
			Error = err
		}

		if Error == nil {
			defer f.Close()

			hashes, err = hash.HashFile(f)
			if err != nil {
				Error = err
			}
		}
	}

	fileInfo := File{
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
	}

	if Error != nil {
		fileInfo.Error = Error.Error()
	}

	return &fileInfo, nil
}

func IndexFileWithoutPermissions(path string, info fs.FileInfo) (*File, error) {
	var fullPath string

	if info.IsDir() {
		fullPath = path
	} else {
		fullPath = fmt.Sprintf("%s/%s", path, info.Name())
	}

	return &File{
		Name:      info.Name(),
		Extension: filepath.Ext(info.Name()),
		Path:      path,
		FullPath:  fullPath,
		Size:      info.Size(),
		IsHidden:  info.Name()[0] == '.',
		IsDir:     info.IsDir(),
		ModTime:   info.ModTime(),
		Error:     ErrNotAllowedToRead.Error(),
	}, nil
}

func IndexDirectory(path string, index *Index) error {
	files, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	var wg = sync.WaitGroup{}

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

// Singleton function to get the index instance
func GetIndexInstance() (*Index, error) {
	m := make(map[string]File)
	// a := []File{}
	once.Do(func() {
		idx = &Index{
			FilesMap: &m,
			// FilesArray: &a,
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
		case "darwin":
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
			if errors.Is(err, fs.ErrNotExist) {
				go func() {
					err := IndexDirectory(rootPath, idx)
					if err != nil {
						log.Println(err)
					}
				}()
			} else {
				ErrIdx = err
				return
			}
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

	i.FilesMapLock.RLock()
	defer i.FilesMapLock.RUnlock()
	startTime := time.Now()
	for _, file := range *i.FilesMap {
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

	log.Printf("Search took %s", time.Since(startTime))
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
		if errors.Is(err, fs.ErrPermission) {
			stats, err := os.Stat(path)
			if err != nil {
				log.Println(err)
				return
			}

			log.Println("Not allowed to read file/folder, indexing without permissions", path)
			file, err := IndexFileWithoutPermissions(path, stats)

			if err != nil {
				log.Println(err)
				return
			}

			err = i.StoreIndex(path, *file)

			if err != nil {
				log.Println(err)
				return
			}

			return
		}
		log.Println(err)
		return
	}

	for _, file := range files {
		filePath := fmt.Sprintf("%s/%s", path, file.Name())

		if file.IsDir() {
			i.FindNewFiles(fmt.Sprintf("%s/%s", path, file.Name()))
			continue
		}

		currFile, err := i.GetIndex(filePath)

		if err != nil {
			continue
		}

		if checksum(filePath, currFile.Hash.MD5) {
			continue
		}

		indexedFile, err := IndexFile(path, file)
		if err != nil {
			log.Println(err)
			continue
		}

		err = i.StoreIndex(filePath, *indexedFile)
		if err != nil {
			log.Println(err)
			continue
		}
	}
}

func (i *Index) CheckForRemovedFiles() {
	var toDelete []string

	i.FilesMapLock.RLock()
	for path, file := range *i.FilesMap {
		if _, err := os.Stat(file.FullPath); os.IsNotExist(err) {
			toDelete = append(toDelete, path)
			log.Printf("File %s has been removed", file.FullPath)
		}
	}
	i.FilesMapLock.RUnlock()

	if len(toDelete) > 0 {
		i.FilesMapLock.Lock()
		for _, path := range toDelete {
			delete(*i.FilesMap, path)
		}
		i.FilesMapLock.Unlock()
	}
}

func (i *Index) StoreIndex(fullPath string, file File) error {
	ok := i.ExistIndex(fullPath)

	if ok {
		return nil
	}

	i.FilesMapLock.Lock()
	(*i.FilesMap)[fullPath] = file
	i.FilesMapLock.Unlock()

	i.newFilesSinceStore++

	return nil
}

func (i *Index) RemoveIndex(path string) error {
	i.FilesMapLock.Lock()
	delete(*i.FilesMap, path)
	i.FilesMapLock.Unlock()

	return nil
}

func (i *Index) GetIndex(path string) (File, error) {
	i.FilesMapLock.RLock()
	defer i.FilesMapLock.RUnlock()

	file, ok := (*i.FilesMap)[path]
	if !ok {
		return File{}, ErrFileNotFound
	}

	return file, nil
}

func (i *Index) ExistIndex(path string) bool {
	i.FilesMapLock.RLock()
	defer i.FilesMapLock.RUnlock()

	_, ok := (*i.FilesMap)[path]
	return ok
}

func (i *Index) GetIndexMap() map[string]File {
	i.FilesMapLock.RLock()
	defer i.FilesMapLock.RUnlock()

	return *i.FilesMap
}

func (i *Index) LoadFileIndex(path string) error {
	i.FilesMapLock.Lock()
	defer i.FilesMapLock.Unlock()

	path = path + IndexFileName

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	lz4Reader := lz4.NewReader(file)

	decoder := json.NewDecoder(lz4Reader)
	err = decoder.Decode(&i.FilesMap)
	if err != nil && err != io.EOF {
		return err
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

	lz4Writer := lz4.NewWriter(file)

	encoder := json.NewEncoder(lz4Writer)
	if err := encoder.Encode(i.FilesMap); err != nil {
		return err
	}

	if err := lz4Writer.Close(); err != nil {
		return err
	}

	i.lastStore = time.Now()
	i.newFilesSinceStore = 0

	return nil
}
