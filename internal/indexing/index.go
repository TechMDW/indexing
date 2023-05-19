package indexing

import (
	"container/heap"
	"context"
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
	"strings"
	"sync"
	"sync/atomic"
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
			PathInfo:              *pathInfo(path),
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
		PathInfo:          *pathInfo(fmt.Sprintf("%s/%s", path, file.Name())),
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

func pathInfo(path string) *PathInfo {
	info := &PathInfo{}

	var err error

	// Abs
	info.Abs, err = filepath.Abs(path)
	if err != nil {
		info.Abs = ""
	}

	// Base
	info.Base = filepath.Base(path)

	// Clean
	info.Clean = filepath.Clean(path)

	// Dir
	info.Dir = filepath.Dir(path)

	// Ext
	info.Ext = filepath.Ext(path)

	// EvalSymlinks
	info.EvalSymlinks, err = filepath.EvalSymlinks(path)
	if err != nil && !os.IsNotExist(err) {
		info.EvalSymlinks = ""
	}

	// IsAbs
	info.IsAbs = filepath.IsAbs(path)

	// VolumeName
	info.VolumeName = filepath.VolumeName(path)

	// Separator
	info.Separator = string(filepath.Separator)

	return info
}

func IndexFileWithoutPermissions(path string, info fs.FileInfo) File {
	var fullPath string

	// TODO: Look into this
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
		PathInfo:          *pathInfo(fullPath),
		Size:              info.Size(),
		IsHidden:          info.Name()[0] == '.',
		IsDir:             info.IsDir(),
		ModTime:           info.ModTime(),
		WindowsAttributes: attr,
		Error:             ErrNotAllowedToRead.Error(),
	}

	return file
}

func IndexFileWithoutInfo(path string) File {
	attr, _ := attributes.GetFileAttributes(path)

	fileInfo := File{
		Path:              path,
		FullPath:          path,
		WindowsAttributes: attr,
		Permissions:       Permissions{},
	}

	return fileInfo
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
		idx = &Index{
			FilesMap:        sync.Map{},
			WindowsDrives:   &[]string{},
			FindNewFilesMap: sync.Map{},
		}

		// Load index from file
		go idx.LoadFileIndex()

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
			// rootPath := "/"
			// go func() {
			// 	idx.FindNewFiles(rootPath)
			// }()
		case "darwin":
			ErrIdx = errors.New("unsupported operating system")
			return
			// rootPath := "/"
			// go func() {
			// 	idx.FindNewFiles(rootPath)
			// }()
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
	var storeFunc func()
	storeFunc = func() {
		var newFilesSinceStore int32
		atomic.StoreInt32(&i.newFilesSinceStore, newFilesSinceStore)
		if newFilesSinceStore >= 50 {
			i.StoreFileIndex()
		} else if time.Since(i.getLastStore()) >= 1*time.Minute && newFilesSinceStore != 0 {
			i.StoreFileIndex()
		}
		time.AfterFunc(10*time.Second, storeFunc)
	}
	go storeFunc()

	var newFilesFunc func()
	newFilesFunc = func() {
		if atomic.LoadInt64(&i.lastFileIndexLoad) == 0 {
			fmt.Println("Loading index from file still in progress...")
			time.AfterFunc(15*time.Second, newFilesFunc)
			return
		}
		oss := runtime.GOOS
		switch oss {
		case "windows":
			sem := make(chan struct{}, 2)
			for _, drive := range *idx.WindowsDrives {
				sem <- struct{}{}
				go func(drive string) {
					defer func() { <-sem }()
					idx.FindNewFiles(drive)
				}(drive)
			}

			for i := 0; i < cap(sem); i++ {
				sem <- struct{}{}
			}
		case "linux":
			idx.FindNewFiles("/")
		case "darwin":
			idx.FindNewFiles("/")
		default:
			log.Println("Unsupported operating system")
			return
		}

		time.AfterFunc(30*time.Second, newFilesFunc)
	}
	go newFilesFunc()

	var removedFilesFunc func()
	removedFilesFunc = func() {
		i.CheckForRemovedFiles()
		time.AfterFunc(5*time.Minute, removedFilesFunc)
	}

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

// Search searches the index based on the query string
//
// It will score the indexes based on the query string and return the top 30 results
func (i *Index) Search(ctx context.Context, q string) []File {
	startTime := time.Now()
	const numWorkers = 100
	results := make([]File, 0, MaxResults)

	pq := NewPriorityQueue(MaxResults)

	filesCh := make(chan File, numWorkers)
	resCh := make(chan File, numWorkers)

	var wg sync.WaitGroup
	for j := 0; j < numWorkers; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range filesCh {
				var scoreTotal int
				var scoreData interface{}

				if file.IsDir {
					scoreTotal, scoreData = ScoreDir(file, q)
				} else {
					scoreTotal, scoreData = ScoreFile(file, q)
				}

				if scoreTotal > 0 {
					file.Internal_metadata.Score = scoreTotal
					file.Internal_metadata.Score_data = scoreData
					select {
					case resCh <- file:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	go func() {
		i.FilesMap.Range(func(key, value interface{}) bool {
			file := value.(File)

			select {
			case filesCh <- file:
			case <-ctx.Done():
				return false
			}

			return true
		})
		close(filesCh)
	}()

	go func() {
		wg.Wait()
		close(resCh)
	}()

	for file := range resCh {
		item := &Item{
			Value:    file,
			Priority: file.Internal_metadata.Score,
		}

		heap.Push(pq, item)
		if pq.Len() > MaxResults {
			heap.Pop(pq)
		}
	}

	for pq.Len() > 0 {
		item := pq.Pop().(*Item)
		results = append(results, item.Value.(File))
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Internal_metadata.Score > results[j].Internal_metadata.Score
	})

	log.Printf("Search took %s", time.Since(startTime))
	return results
}

// FindNewFiles finds new files in a directory and stores them in the FilesMap
func (i *Index) FindNewFiles(path string) {
	// TODO: Issues with onedrive
	if strings.Contains(path, "OneDrive") {
		return
	}

	if isBlacklisted(path) {
		return
	}

	// TODO: Not sure this is the best way
	// Check if this path is already being processed
	if _, ok := i.FindNewFilesMap.Load(path); ok {
		return
	}

	// Mark this path as being processed
	i.FindNewFilesMap.Store(path, struct{}{})

	defer func(path string) {
		// Mark this path as no longer being processed
		i.FindNewFilesMap.Delete(path)
	}(path)

	files, err := os.ReadDir(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Println(err)
			return
		}

		info, err := os.Stat(path)
		if err != nil {
			indexedFile := IndexFileWithoutInfo(path)

			err = i.StoreIndex(path, indexedFile)

			if err != nil {
				log.Println(err)
				return
			}
			return
		}

		file := IndexFileWithoutPermissions(path, info)

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

			// TODO: Add back the checksum, maybe?
			// For now we just check the mod time and size
			info, err := file.Info()
			if err != nil {
				log.Println(err)
				return
			}
			if currFile.ModTime == info.ModTime() && currFile.Size == info.Size() {
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

// CheckForRemovedFiles checks if any files have been removed from the index
func (i *Index) CheckForRemovedFiles() {
	const workers = 4
	pathsCh := make(chan string)
	toDelete := make(chan string)

	go func() {
		i.FilesMap.Range(func(key, value interface{}) bool {
			file := value.(File)
			pathsCh <- file.FullPath
			return true
		})
		close(pathsCh)
	}()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range pathsCh {
				if _, err := os.Stat(path); os.IsNotExist(err) {
					toDelete <- path
				}

				if isBlacklisted(path) {
					toDelete <- path
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(toDelete)
	}()

	var toDeleteSlice []string
	for path := range toDelete {
		toDeleteSlice = append(toDeleteSlice, path)
	}

	for _, path := range toDeleteSlice {
		i.RemoveIndex(path)
	}
}

// StoreIndex stores a File in the FilesMap
func (i *Index) StoreIndex(fullPath string, file File) error {
	ok := i.ExistIndex(fullPath)

	if ok {
		return nil
	}

	i.FilesMap.Store(fullPath, file)

	atomic.AddInt32(&i.newFilesSinceStore, 1)

	return nil
}

// RemoveIndex removes a File from the FilesMap
func (i *Index) RemoveIndex(key string) error {
	i.FilesMap.Delete(key)

	return nil
}

// GetIndex returns a File from the FilesMap
func (i *Index) GetIndex(key string) (File, error) {

	val, ok := i.FilesMap.Load(key)

	if !ok {
		return File{}, ErrFileNotFound
	}

	file, ok := val.(File)
	if !ok {
		return File{}, ErrFileNotFound
	}

	return file, nil
}

// ExistIndex checks if a key exists in the FilesMap
func (i *Index) ExistIndex(key string) bool {
	_, ok := i.FilesMap.Load(key)
	return ok
}

// LoadFileIndex reads the FilesMap from disk in NDJSON format.
//
// TODO: Make this function more robust. Currently it can take a long time to load the index from disk.
func (i *Index) LoadFileIndex() error {
	startTime := time.Now()
	path, err := getTechMDWDir()

	if err != nil {
		return err
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			atomic.StoreInt64(&i.lastFileIndexLoad, time.Now().Unix())
		}
		return err
	}
	defer file.Close()

	lz4Reader := lz4.NewReader(file)
	decoder := json.NewDecoder(lz4Reader)

	for {
		var entry struct {
			Key   string
			Value File
		}

		err := decoder.Decode(&entry)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		i.FilesMap.Store(entry.Key, entry.Value)
	}

	atomic.StoreInt64(&i.lastFileIndexLoad, time.Now().Unix())

	log.Printf("Loaded index from file in %s", time.Since(startTime))
	return nil
}

// Update the last time the index was stored to disk
func (i *Index) updateLastStore() {
	atomic.StoreInt64(&i.lastStore, time.Now().Unix())
	atomic.StoreInt32(&i.newFilesSinceStore, 0)
}

// Get the last time the index was stored to disk
func (i *Index) getLastStore() time.Time {
	return time.Unix(atomic.LoadInt64(&i.lastStore), 0)
}

// Create a copy of the FilesMap and store it to disk
func (i *Index) StoreFileIndex() error {
	g := graceful.Shutdown()
	g.AddTask()
	defer g.DoneTask()

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
	defer lz4Writer.Close()

	encoder := json.NewEncoder(lz4Writer)

	i.FilesMap.Range(func(key, value interface{}) bool {
		entry := struct {
			Key   string
			Value File
		}{
			Key:   key.(string),
			Value: value.(File),
		}

		if err := encoder.Encode(entry); err != nil {
			log.Println(err)
			return false
		}

		return true
	})

	i.updateLastStore()

	return nil
}
