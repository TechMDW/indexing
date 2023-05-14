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
	Score       int         `json:"score"`
}

type Hash struct {
	MD5    string `json:"MD5"`
	SHA1   string `json:"SHA1"`
	SHA256 string `json:"SHA256"`
	SHA512 string `json:"SHA512"`
	SHA3   SHA3   `json:"SHA3"`
	CRC32  string `json:"CRC32"`
	CRC64  string `json:"CRC64"`
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
