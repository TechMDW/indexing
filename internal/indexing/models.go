package indexing

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/TechMDW/indexing/internal/attributes"
	"github.com/TechMDW/indexing/internal/hash"
)

type Index struct {
	FilesMap           sync.Map     `json:"files"`
	WindowsDrivesLock  sync.RWMutex `json:"-"`
	WindowsDrives      *[]string    `json:"windowsDrivesArray"`
	FindNewFilesMap    sync.Map     `json:"-"`
	lastFileIndexLoad  int64
	newFilesSinceStore int32
	lastStore          int64
}

type File struct {
	Name                  string                       `json:"name"`
	Extension             string                       `json:"ext"`
	Path                  string                       `json:"path"`
	FullPath              string                       `json:"fullPath"`
	PathInfo              PathInfo                     `json:"pathInfo"`
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
	Internal_metadata     internal_metadata
}

type internal_metadata struct {
	Score      int
	Score_data interface{}
}

type Permissions struct {
	Owner      string      `json:"owner"`
	Group      string      `json:"group"`
	Other      string      `json:"other"`
	Permission os.FileMode `json:"permission"`
}

type PathInfo struct {
	Abs          string `json:"abs"`
	Base         string `json:"base"`
	Clean        string `json:"clean"`
	Dir          string `json:"dir"`
	Ext          string `json:"ext"`
	EvalSymlinks string `json:"evalSymlinks"`
	IsAbs        bool   `json:"isAbs"`
	VolumeName   string `json:"volumeName"`
	Separator    string `json:"separator"`
}

func getTechMDWDir() (string, error) {
	path, err := os.UserConfigDir()

	if err != nil {
		return "", err
	}

	goDownHistoryPath := filepath.Join(path, "TechMDW", "indexing", IndexFileName)

	return goDownHistoryPath, nil
}

func isBlacklisted(path string) bool {
	for _, b := range blacklist {
		match, err := regexp.MatchString(b, filepath.Clean(path))
		if err != nil {
			fmt.Printf("Invalid regex pattern: %v\n", err)
			return false
		}
		if match {
			return true
		}
	}
	return false
}
