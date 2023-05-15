package indexing

import "errors"

var (
	ErrFileNotFound = errors.New("file not found")
)

const (
	IndexFileName = "TechMDW_Indexing.json"
)

const (
	MaxGoRoutines = 15

	MaxResults = 500
)
