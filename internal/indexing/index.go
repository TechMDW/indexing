package indexing

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/TechMDW/indexing/internal/attributes"
	"github.com/TechMDW/indexing/internal/graceful"
	"github.com/TechMDW/indexing/internal/hash"

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
	windowsAttr, err := attributes.GetFileAttributes(path)

	if windowsAttr.OneDrive || err != nil {
		fileInfo := File{
			Name:                  file.Name(),
			Extension:             filepath.Ext(file.Name()),
			Path:                  path,
			FullPath:              fmt.Sprintf("%s/%s", path, file.Name()),
			IsHidden:              file.Name()[0] == '.',
			IsDir:                 file.IsDir(),
			WindowsAttributes:     windowsAttr,
			IsOneDrivePlaceholder: true,
			Permissions:           Permissions{},
		}

		return &fileInfo, nil
	}

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
		defer f.Close()

		if Error == nil {
			hashes, err = hash.HashFile(f)
			if err != nil {
				Error = err
			}
		}
	}

	fileInfo := File{
		Name:              file.Name(),
		Extension:         filepath.Ext(file.Name()),
		Path:              path,
		FullPath:          fmt.Sprintf("%s/%s", path, file.Name()),
		Size:              info.Size(),
		IsHidden:          file.Name()[0] == '.',
		IsDir:             file.IsDir(),
		ModTime:           info.ModTime(),
		WindowsAttributes: windowsAttr,
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

func IndexFileWithoutPermissions(path string, info fs.FileInfo) File {
	var fullPath string

	if info.IsDir() {
		fullPath = path
	} else {
		fullPath = fmt.Sprintf("%s/%s", path, info.Name())
	}

	attr, _ := attributes.GetFileAttributes(path)

	file := File{
		Name:              info.Name(),
		Extension:         filepath.Ext(info.Name()),
		Path:              path,
		FullPath:          fullPath,
		Size:              info.Size(),
		IsHidden:          info.Name()[0] == '.',
		IsDir:             info.IsDir(),
		ModTime:           info.ModTime(),
		WindowsAttributes: attr,
		Error:             ErrNotAllowedToRead.Error(),
	}

	return file
}

// DEPRECATED
func IndexDirectory(path string, index *Index) error {
	files, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	var wg = sync.WaitGroup{}

	for _, file := range files {
		lim <- struct{}{}
		wg.Add(1)
		go func(file fs.DirEntry) {
			defer wg.Done()
			defer func() { <-lim }()

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
		m := make(map[string]File)

		idx = &Index{
			FilesMap:        &m,
			WindowsDrives:   &[]string{},
			FindNewFilesMap: &map[string]struct{}{},
		}

		// Load index from file
		idx.LoadFileIndex()

		// Get windows or linux
		oss := runtime.GOOS

		fmt.Println("Operating system:", oss)

		switch oss {
		case "windows":
			// Loop through all possible drives on Windows
			for _, driveLetter := range WIN_PossibleDriveLetters {
				drivePath := fmt.Sprintf("%s:/", string(driveLetter))
				if _, err := os.Stat(drivePath); !os.IsNotExist(err) {
					log.Printf("Found drive %s\n", drivePath)

					idx.WindowsDrivesLock.Lock()
					*idx.WindowsDrives = append(*idx.WindowsDrives, drivePath)
					idx.WindowsDrivesLock.Unlock()
				}
			}
		case "linux":
			ErrIdx = errors.New("unsupported operating system")
			return
			rootPath := "/"
			go func() {
				idx.FindNewFiles(rootPath)
			}()
		case "darwin":
			ErrIdx = errors.New("unsupported operating system")
			return
			rootPath := "/"
			go func() {
				idx.FindNewFiles(rootPath)
			}()
		default:
			ErrIdx = errors.New("unsupported operating system")
			return
		}

		log.Println("Starting handler")
		go idx.handler()
	})

	return idx, ErrIdx
}

// TODO: Don't like this function...
func (i *Index) handler() {
	var autoStoreFunc func()
	autoStoreFunc = func() {
		if i.newFilesSinceStore != 0 {
			i.StoreFileIndex()
		}
		time.AfterFunc(1*time.Minute, autoStoreFunc)
	}
	go autoStoreFunc()

	var storeFunc func()
	storeFunc = func() {
		if i.newFilesSinceStore >= 50 {
			i.StoreFileIndex()
		} else if time.Since(i.lastStore) >= 1*time.Minute && i.newFilesSinceStore != 0 {
			i.StoreFileIndex()
		}
		time.AfterFunc(5*time.Second, storeFunc)
	}
	go storeFunc()

	var newFilesFunc func()
	newFilesFunc = func() {
		if runtime.GOOS == "windows" {
			for _, drive := range *idx.WindowsDrives {
				idx.FindNewFiles(drive)
			}
		} else if runtime.GOOS == "linux" {
			idx.FindNewFiles("/")
		} else if runtime.GOOS == "darwin" {
			idx.FindNewFiles("/")
		}

		time.AfterFunc(30*time.Second, newFilesFunc)
	}

	var removedFilesFunc func()
	removedFilesFunc = func() {
		i.CheckForRemovedFiles()
		time.AfterFunc(15*time.Second, removedFilesFunc)
	}
	go newFilesFunc()

	var checkForNewDrives func()
	checkForNewDrives = func() {
		if runtime.GOOS == "windows" {
			for _, driveLetter := range WIN_PossibleDriveLetters {
				drivePath := fmt.Sprintf("%s:/", string(driveLetter))
				if _, err := os.Stat(drivePath); !os.IsNotExist(err) {
					var found bool
					for _, drive := range *idx.WindowsDrives {
						if drive == drivePath {
							found = true
							break
						}
					}

					if !found {
						log.Println("Found new drive", drivePath)
						idx.WindowsDrivesLock.Lock()
						*idx.WindowsDrives = append(*idx.WindowsDrives, drivePath)
						idx.WindowsDrivesLock.Unlock()
						continue
					}
				}
			}
		}
		time.AfterFunc(10*time.Second, checkForNewDrives)
	}

	delayRemovedFiles := time.NewTimer(30 * time.Second)
	delayCheckForNewDrives := time.NewTimer(30 * time.Second)

	for {
		select {
		case <-delayRemovedFiles.C:
			go removedFilesFunc()
		case <-delayCheckForNewDrives.C:
			go checkForNewDrives()
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
	// TODO: Not sure this is the best way
	// Check if this path is already being processed
	i.FindNewFilesMapLock.Lock()
	if _, ok := (*i.FindNewFilesMap)[path]; ok {
		i.FindNewFilesMapLock.Unlock()
		return
	}

	// Mark this path as being processed
	(*i.FindNewFilesMap)[path] = struct{}{}
	i.FindNewFilesMapLock.Unlock()

	defer func() {
		// Mark this path as no longer being processed
		i.FindNewFilesMapLock.Lock()
		delete(*i.FindNewFilesMap, path)
		i.FindNewFilesMapLock.Unlock()
	}()

	files, err := os.ReadDir(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Println(err)
			return
		}

		stats, err := os.Stat(path)
		if err != nil {
			log.Println(err)
			return
		}

		file := IndexFileWithoutPermissions(path, stats)

		err = i.StoreIndex(path, file)

		if err != nil {
			return
		}

		return
	}

	wg := sync.WaitGroup{}

	for _, file := range files {
		lim <- struct{}{}
		wg.Add(1)
		go func(file fs.DirEntry) {
			defer wg.Done()
			defer func() { <-lim }()

			filePath := fmt.Sprintf("%s/%s", path, file.Name())

			if file.IsDir() {
				f, err := IndexFile(path, file)

				if err != nil {
					return
				}

				err = i.StoreIndex(filePath, *f)

				if err != nil {
					return
				}

				go i.FindNewFiles(filePath)
				return
			}

			currFile, err := i.GetIndex(filePath)

			if err != nil {
				if errors.Is(err, ErrFileNotFound) {
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

					return
				}
				log.Println(err)
				return
			}

			if currFile.Error != "" {
				return
			}

			if checksum(filePath, currFile.Hash.MD5) {
				return
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

		}(file)
	}

	wg.Wait()
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

func (i *Index) LoadFileIndex() error {
	i.FilesMapLock.Lock()
	defer i.FilesMapLock.Unlock()

	path, err := getTechMDWDir()

	if err != nil {
		return err
	}

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

func (i *Index) StoreFileIndex() error {
	g := graceful.Shutdown()
	g.AddTask()
	defer g.DoneTask()

	i.FilesMapLock.RLock()
	defer i.FilesMapLock.RUnlock()

	path, err := getTechMDWDir()

	if err != nil {
		log.Println(err)
		return err
	}

	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			log.Println(err)
			return err
		}
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Println(err)
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
