package indexing

import "errors"

var (
	ErrFileNotFound = errors.New("file not found")

	ErrNotAllowedToRead = errors.New("not allowed to read file/folder")
)

const (
	IndexFileName = ".index.json.lz4"
)

const (
	MaxGoRoutines = 5

	MaxResults = 30

	WIN_PossibleDriveLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
)
