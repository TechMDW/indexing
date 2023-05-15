package indexing

import (
	"fmt"
	"strings"
)

type FileScore struct {
	Contains   int
	Exact      int
	Start      int
	Word       int
	Length     int
	Extension  int
	Hash       int
	Permission int
}

func ScoreFile(file File, query string) (int, FileScore) {
	var score FileScore

	lQuery := strings.ToLower(query)
	lName := strings.ToLower(file.Name)
	lPermissions := strings.ToLower(file.Permissions.Permission.String())

	if strings.Contains(lName, lQuery) {
		score.Contains += 2

		// If the query matches the permissions exactly (case sensitive)
		if strings.Contains(file.Name, query) {
			score.Exact += 2
		}

		// If the query is at the start of the file name
		if strings.Index(file.Name, query) == 0 {
			score.Start += 2
		}

		// If the query is at the start of the file name
		if strings.Index(lName, lQuery) == 0 {
			score.Start += 2
		}

		// If the query is a word in the file name
		for _, word := range strings.Split(file.Name, " ") {
			if word == query {
				score.Word += 3
			}
		}

		// Add a score boost proportional to the length of the match
		matchRatio := float64(len(query)) / float64(len(file.Name))
		score.Length += int(matchRatio * 10)

		// If the query is a file extension
		if strings.Index(query, ".") == 0 {
			spl := strings.Split(file.Name, ".")
			ext := spl[len(spl)-1]

			if strings.HasSuffix(file.Name, query) {
				score.Extension += 5
			} else if strings.Index(fmt.Sprintf(".%s", ext), query) == 0 {
				score.Extension += 3
			} else if strings.Index(fmt.Sprintf(".%s", strings.ToLower(ext)), lQuery) == 0 {
				score.Extension += 2
			}
		}
	}

	if strings.Contains(lPermissions, lQuery) {
		score.Permission += 2

		// If the query matches the permissions exactly (case sensitive)
		if strings.Contains(file.Permissions.Permission.String(), query) {
			score.Permission += 2
		}

		// If the query is at the start of the file name
		if strings.Index(file.Permissions.Permission.String(), query) == 0 {
			score.Permission += 2
		}

		// If the query is a word in the file name
		for _, word := range strings.Split(file.Permissions.Permission.String(), " ") {
			if word == query {
				score.Permission += 3
			}
		}
	}

	hashScore := 2
	if !file.IsDir && (strings.Contains(file.Hash.MD5, query) ||
		strings.Contains(file.Hash.SHA1, query) ||
		strings.Contains(file.Hash.SHA2.SHA256, query) ||
		strings.Contains(file.Hash.SHA2.SHA512, query) ||
		strings.Contains(file.Hash.SHA3.SHA256, query) ||
		strings.Contains(file.Hash.SHA3.SHA512, query) ||
		strings.Contains(file.Hash.CRC.CRC32, query) ||
		strings.Contains(file.Hash.CRC.CRC64, query)) {
		score.Hash += hashScore
	}

	return score.Contains + score.Exact + score.Start + score.Word + score.Length + score.Extension + score.Hash, score
}
