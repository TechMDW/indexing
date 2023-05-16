package indexing

import "errors"

var (
	ErrFileNotFound = errors.New("file not found")

	ErrNotAllowedToRead = errors.New("not allowed to read file/folder")
)

const (
	IndexFileName = ".TechMDW_Indexing.json.lz4"
)

const (
	MaxGoRoutines = 15

	MaxResults = 500
)
