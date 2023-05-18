package indexing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
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
	var autoStoreFunc func()
	autoStoreFunc = func() {
		if atomic.LoadInt32(&i.newFilesSinceStore) != 0 {
			i.StoreFileIndex()
		}
		time.AfterFunc(1*time.Minute, autoStoreFunc)
	}
	go autoStoreFunc()

	var storeFunc func()
	storeFunc = func() {
		var newFilesSinceStore int32
		atomic.StoreInt32(&i.newFilesSinceStore, newFilesSinceStore)
		if newFilesSinceStore >= 50 {
			i.StoreFileIndex()
		} else if time.Since(i.getLastStore()) >= 1*time.Minute && newFilesSinceStore != 0 {
			i.StoreFileIndex()
		}
		time.AfterFunc(5*time.Second, storeFunc)
	}
	go storeFunc()

	var newFilesFunc func()
	newFilesFunc = func() {
		if atomic.LoadInt64(&i.lastFileIndexLoad) == 0 {
			fmt.Println("Loading index from file still in progress...")
			time.AfterFunc(30*time.Second, newFilesFunc)
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
		time.AfterFunc(2*time.Minute, removedFilesFunc)
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

func (i *Index) Search(ctx context.Context, q string) []File {
	startTime := time.Now()
	const numWorkers = 30
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
			value:    file,
			priority: file.Internal_metadata.Score,
		}

		if pq.Len() < MaxResults {
			pq.Push(item)
		} else if top := (*pq)[0]; file.Internal_metadata.Score > top.priority {
			pq.Pop()
			pq.Push(item)
		}
	}

	for pq.Len() > 0 {
		item := pq.Pop().(*Item)
		results = append(results, item.value)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Internal_metadata.Score > results[j].Internal_metadata.Score
	})

	log.Printf("Search took %s", time.Since(startTime))
	return results
}

func (i *Index) FindNewFiles(path string) {
	// TODO: Issues with onedrive
	if strings.Contains(path, "OneDrive") {
		return
	}
	// TODO: Check for a better way to do this
	// Ignore temp folders to avoid indexing temp files. This will speed things up
	tempDirRegex := regexp.MustCompile(`(?i)([/\\](temp|tmp|\.tmp)[/\\]|^temp[/\\]|^tmp[/\\]|^\.tmp[/\\])`)
	if tempDirRegex.MatchString(path) {
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

func (i *Index) StoreIndex(fullPath string, file File) error {
	ok := i.ExistIndex(fullPath)

	if ok {
		return nil
	}

	i.FilesMap.Store(fullPath, file)

	atomic.AddInt32(&i.newFilesSinceStore, 1)

	return nil
}

func (i *Index) RemoveIndex(key string) error {
	i.FilesMap.Delete(key)

	return nil
}

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

func (i *Index) ExistIndex(key string) bool {
	_, ok := i.FilesMap.Load(key)
	return ok
}

// LoadFileIndex reads the FilesMap from disk in NDJSON format.
func (i *Index) LoadFileIndex() error {
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

	return nil
}

func (i *Index) updateLastStore() {
	atomic.StoreInt64(&i.lastStore, time.Now().Unix())
	atomic.StoreInt32(&i.newFilesSinceStore, 0)
}

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
