package indexing

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TechMDW/indexing/internal/attributes"
	"github.com/TechMDW/indexing/internal/hash"
)

type Index struct {
	FilesMapLock        sync.RWMutex         `json:"-"`
	FilesMap            *map[string]File     `json:"files"`
	FilesArrayLock      sync.RWMutex         `json:"-"`
	FilesArray          *[]File              `json:"filesArray"`
	WindowsDrivesLock   sync.RWMutex         `json:"-"`
	WindowsDrives       *[]string            `json:"windowsDrivesArray"`
	FindNewFilesMap     *map[string]struct{} `json:"-"`
	FindNewFilesMapLock sync.RWMutex         `json:"-"`
	newFilesSinceStore  int32
	lastStore           int64
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
	score      int
	score_data interface{}
}

type Permissions struct {
	Owner      string      `json:"owner"`
	Group      string      `json:"group"`
	Other      string      `json:"other"`
	Permission os.FileMode `json:"permission"`
}

func getTechMDWDir() (string, error) {
	path, err := os.UserConfigDir()

	if err != nil {
		return "", err
	}

	goDownHistoryPath := filepath.Join(path, "TechMDW", "indexing", IndexFileName)

	return goDownHistoryPath, nil
}
