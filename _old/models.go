package main

import (
	"os"
	"sync"
	"time"
)

type FileInfo struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Size        int64       `json:"size"`
	IsDir       bool        `json:"isDir"`
	ModTime     time.Time   `json:"modTime"`
	Permissions os.FileMode `json:"permissions"`
	Hash        Hash        `json:"hash"`
	Score       int         `json:"score,omitempty"`
	FileScore   FileScore   `json:"fileScore,omitempty"`
	DirScore    DirScore    `json:"dirScore,omitempty"`
}

type SHA3 struct {
	SHA256 string `json:"SHA256"`
	SHA512 string `json:"SHA512"`
}

var fileIndex []FileInfo
var fileIndexMutex sync.Mutex

var history chan string

const maxHistory = 100
const maxcurrent = 30
