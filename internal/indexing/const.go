package indexing

import "errors"

var (
	ErrFileNotFound = errors.New("file not found")

	ErrNotAllowedToRead = errors.New("not allowed to read file/folder")
)

const (
	IndexFileName = ".index.ndjson.lz4"
)

const (
	MaxGoRoutines = 5

	MaxResults = 30

	WIN_PossibleDriveLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

// Default blacklist
var blacklist = []string{
	`C:\\Windows.*`,
	`C:\\Program Files.*`,         // TODO: Maybe
	`C:\\Program Files \(x86\).*`, // TODO: Maybe
	`C:\\Recovery.*`,
	`C:\\System Volume Information.*`,
	`.*pagefile\.sys`,
	`.*hiberfil\.sys`,
	`.*swapfile\.sys`,
	// `C:\\Users\\.*\\AppData.*`, // TODO: Maybe
	`C:\\Windows\\Temp.*`,
	`C:\\Users\\.*\\AppData\\Local\\Temp.*`,
	`.*\\Temp.*`,
	`.*\\temp.*`,
}
