package indexing

import (
	"regexp"
	"strings"
)

type DirScore struct {
	Contains    int
	Exact       int
	Start       int
	Word        int
	Length      int
	Permissions int
}

func ScoreDir(file File, query string) (int, DirScore) {
	var score DirScore

	lQuery := strings.ToLower(query)
	lPath := strings.ToLower(file.Path)
	lPermissions := strings.ToLower(file.Permissions.Permission.String())

	if strings.Contains(lPath, lQuery) {
		score.Contains += 2

		// If the query matches the path (case sensitive)
		if strings.Contains(file.Path, query) {
			score.Exact += 2
		}

		// If the query is at the start of the file name
		if strings.Index(file.Path, query) == 0 {
			score.Start += 2
		}

		re := regexp.MustCompile(`[\/\\](\w+)([\/\\]|$)`)
		matches := re.FindAllStringSubmatch(file.Path, -1)

		for _, match := range matches {
			if strings.Contains(match[1], query) {
				score.Word += 3
			}
		}

		// Add a score boost proportional to how far down the path the match is
		if strings.Index(query, "/") == 0 || strings.Index(query, "\\") == 0 {
			paths := strings.Split(file.Path, "/")
			queryPaths := strings.Split(query, "/")

			for i, path := range paths {
				// Check if we have enough parts in the query to compare
				if len(queryPaths) <= i {
					break
				}

				if path == queryPaths[i] {
					score.Length += 2
				}

				if strings.Contains(path, queryPaths[i]) {
					score.Length += 1
				}
			}
		}
	}

	if strings.Contains(lPermissions, lQuery) {
		score.Contains += 2

		// If the query matches the permissions exactly (case sensitive)
		if strings.Contains(file.Permissions.Permission.String(), query) {
			score.Exact += 2
		}

		// If the query is at the start of the file name
		if strings.Index(file.Permissions.Permission.String(), query) == 0 {
			score.Start += 2
		}

		// If the query is a word in the file name
		for _, word := range strings.Split(file.Permissions.Permission.String(), " ") {
			if word == query {
				score.Word += 3
			}
		}
	}

	return score.Contains + score.Exact + score.Start + score.Word + score.Length, score
}
