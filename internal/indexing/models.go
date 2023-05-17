package indexing

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/TechMDW/indexing/internal/attributes"
	"github.com/TechMDW/indexing/internal/hash"
)

type Index struct {
	FilesMapLock       sync.RWMutex     `json:"-"`
	FilesMap           *map[string]File `json:"files"`
	FilesArrayLock     sync.RWMutex     `json:"-"`
	FilesArray         *[]File          `json:"filesArray"`
	rootPath           string
	newFilesSinceStore int
	lastStore          time.Time
}

type File struct {
	Name                  string                       `json:"name"`
	Extension             string                       `json:"ext"`
	Path                  string                       `json:"path"`
	FullPath              string                       `json:"fullPath"`
	Size                  int64                        `json:"size"`
	IsHidden              bool                         `json:"isHidden"`
	IsDir                 bool                         `json:"isDir"`
	IsOneDrivePlaceholder bool                         `json:"isOneDrive"`
	CreatedTime           time.Time                    `json:"created"`
	ModTime               time.Time                    `json:"modTime"`
	AccessedTime          time.Time                    `json:"accessed"`
	Permissions           Permissions                  `json:"permissions"`
	Hash                  hash.Hash                    `json:"hash"`
	Error                 string                       `json:"error,omitempty"`
	WindowsAttributes     attributes.WindowsAttributes `json:"windowsAttributes,omitempty"`
	internal_metadata     internal_metadata
}

type internal_metadata struct {
	score      int         `json:"score,omitempty"`
	score_data interface{} `json:"score_data,omitempty"`
}

type Permissions struct {
	Owner      string      `json:"owner"`
	Group      string      `json:"group"`
	Other      string      `json:"other"`
	Permission os.FileMode `json:"permission"`
}

func checksum(filePath string, hash string) bool {
	file, err := os.Open(filePath)

	if err != nil {
		fmt.Println(err)
		return false
	}

	defer file.Close()

	hasher := md5.New()

	_, err = io.Copy(hasher, file)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)) == hash
}
