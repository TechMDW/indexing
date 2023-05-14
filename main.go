package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash/crc32"
	"hash/crc64"
	"io"
	"log"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/asticode/go-astikit"
	"github.com/asticode/go-astilectron"
	"golang.org/x/crypto/sha3"
)

func main() {
	a, _ := astilectron.New(nil, astilectron.Options{
		AppName: "Indexer",
	})

	defer a.Close()

	// Start astilectron
	a.Start()

	startWindow(a)

	// Start history channel
	history = make(chan string, maxHistory)

	rootDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// Get windows or linux
	oss := runtime.GOOS

	switch oss {
	case "windows":
		rootDir = strings.Split(rootDir, ":")[0] + ":/"
	case "linux":
		if rootDir == "/" {
			break
		}

		split := strings.Split(rootDir, "/")
		rootDir = fmt.Sprintf("/%s", split[1])
	default:
	}

	history <- fmt.Sprintf("Indexing directory: %s", rootDir)
	// Load the file index
	err = loadFileIndex(rootDir)
	if err != nil {
		log.Println(err)
	}

	// Start the auto store goroutine
	go autoStore(rootDir)

	// Index the current directory
	err = indexDirectory(rootDir)
	if err != nil {
		log.Fatal(err)
	}

	err = storeIndex(rootDir)

	if err != nil {
		log.Fatal(err)
	}

	a.Wait()
}

func startWindow(a *astilectron.Astilectron) {
	w, err := a.NewWindow("./home.html", &astilectron.WindowOptions{
		Center: astikit.BoolPtr(true),
		Height: astikit.IntPtr(600),
		Width:  astikit.IntPtr(1000),
	})

	if err != nil {
		log.Fatal(err)
	}

	w.Create()
	w.OpenDevTools()
	listenForInput(w)
}

func listenForInput(w *astilectron.Window) {
	w.OnMessage(func(m *astilectron.EventMessage) interface{} {
		// Unmarshal
		var s string
		m.Unmarshal(&s)

		files := searchIndex(s)

		w.SendMessage(files)
		return nil
	})
}

func scoreFile(file FileInfo, query string) int {
	score := 0

	lQuery := strings.ToLower(query)
	lName := strings.ToLower(file.Name)
	lPath := strings.ToLower(file.Path)
	lPermissions := strings.ToLower(file.Permissions.String())

	if !file.IsDir && strings.Contains(lName, lQuery) {
		score += 3

		// If the query matches the permissions exactly (case sensitive)
		if strings.Contains(file.Name, query) {
			score += 3
		}

		// If the query is at the start of the file name
		if strings.Index(file.Name, query) == 0 {
			score += 3
		}

		// If the query is a word in the file name
		for _, word := range strings.Split(file.Name, " ") {
			if word == query {
				score += 4
			}
		}

		// Add a score boost proportional to the length of the match
		matchRatio := float64(len(query)) / float64(len(file.Name))
		score += int(matchRatio * 3)

		// If the query is a file extension
		if strings.Index(query, ".") == 0 {
			spl := strings.Split(file.Name, ".")
			ext := spl[len(spl)-1]

			if strings.HasSuffix(file.Name, query) {
				score += 10
			} else if strings.Index(fmt.Sprintf(".%s", ext), query) == 0 {
				score += 5
			} else if strings.Index(fmt.Sprintf(".%s", strings.ToLower(ext)), lQuery) == 0 {
				score += 3
			}
		}
	}

	if file.IsDir && strings.Contains(lPath, lQuery) {
		score += 2

		// If the query matches the path (case sensitive)
		if strings.Contains(file.Path, query) {
			score += 2
		}

		// If the query is at the start of the file name
		if strings.Index(file.Path, query) == 0 {
			score += 3
		}

		re := regexp.MustCompile(`[\/\\](\w+)([\/\\]|$)`)
		matches := re.FindAllStringSubmatch(file.Path, -1)

		for _, match := range matches {
			if strings.Contains(match[1], query) {
				score += 3
			}
		}

		// Add a score boost proportional to how far down the path the match is
		if (strings.Index(query, "/") == 0 || strings.Index(query, "\\") == 0) && file.IsDir {
			paths := strings.Split(file.Path, "/")
			queryPaths := strings.Split(query, "/")

			fmt.Println(paths, queryPaths)
			for i, path := range paths {
				// Check if we have enough parts in the query to compare
				if len(queryPaths) <= i {
					break
				}

				if path == queryPaths[i] {
					score += (i + 1) * 10
				}

				if strings.Contains(path, queryPaths[i]) {
					score += (i + 1) * 6
				}
			}
		}
	}

	if strings.Contains(lPermissions, lQuery) {
		score += 2

		// If the query matches the permissions exactly (case sensitive)
		if strings.Contains(file.Permissions.String(), query) {
			score += 2
		}

		// If the query is at the start of the file name
		if strings.Index(file.Permissions.String(), query) == 0 {
			score += 2
		}

		// If the query is a word in the file name
		for _, word := range strings.Split(file.Permissions.String(), " ") {
			if word == query {
				score += 3
			}
		}
	}

	hashScore := 2
	if !file.IsDir && (strings.Contains(file.Hash.MD5, query) ||
		strings.Contains(file.Hash.SHA1, query) ||
		strings.Contains(file.Hash.SHA256, query) ||
		strings.Contains(file.Hash.SHA512, query) ||
		strings.Contains(file.Hash.SHA3.SHA256, query) ||
		strings.Contains(file.Hash.SHA3.SHA512, query) ||
		strings.Contains(file.Hash.CRC32, query) ||
		strings.Contains(file.Hash.CRC64, query)) {
		score += hashScore
	}

	return score
}

func searchIndex(query string) []FileInfo {
	type scoredFile struct {
		file  FileInfo
		score int
	}
	var scoredFiles []scoredFile
	seen := make(map[string]bool)

	for _, file := range fileIndex {
		if seen[file.Path] {
			continue
		}

		score := scoreFile(file, query)
		if score > 0 {
			scoredFiles = append(scoredFiles, scoredFile{file, score})
			seen[file.Path] = true
		}
	}

	sort.Slice(scoredFiles, func(i, j int) bool {
		return scoredFiles[i].score > scoredFiles[j].score
	})

	var results []FileInfo
	for _, scoredFile := range scoredFiles {
		scoredFile.file.Score = scoredFile.score
		results = append(results, scoredFile.file)
	}

	return results
}

func hashFile(filePath string) (Hash, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return Hash{}, err
	}

	defer file.Close()

	// MD5
	hasherMD5 := md5.New()

	// SHA1
	hasherSHA1 := sha1.New()

	// SHA2
	hasherSHA256 := sha256.New()
	hasherSHA512 := sha512.New()

	// SHA3
	hasherSHA3_256 := sha3.New256()
	hasherSHA3_512 := sha3.New512()

	// CRC
	crc32Hasher := crc32.NewIEEE()
	crc64Hasher := crc64.New(crc64.MakeTable(crc64.ECMA))

	multiWriter := io.MultiWriter(hasherSHA1, hasherMD5, hasherSHA256, hasherSHA512, hasherSHA3_256, hasherSHA3_512, crc32Hasher, crc64Hasher)

	_, err = io.Copy(multiWriter, file)
	if err != nil {
		return Hash{}, err
	}

	hash := Hash{
		MD5:    fmt.Sprintf("%x", hasherMD5.Sum(nil)),
		SHA1:   fmt.Sprintf("%x", hasherSHA1.Sum(nil)),
		SHA256: fmt.Sprintf("%x", hasherSHA256.Sum(nil)),
		SHA512: fmt.Sprintf("%x", hasherSHA512.Sum(nil)),
		SHA3: SHA3{
			SHA256: fmt.Sprintf("%x", hasherSHA3_256.Sum(nil)),
			SHA512: fmt.Sprintf("%x", hasherSHA3_512.Sum(nil)),
		},
		CRC32: fmt.Sprintf("%x", crc32Hasher.Sum(nil)),
		CRC64: fmt.Sprintf("%x", crc64Hasher.Sum(nil)),
	}

	return hash, nil
}

func checksum(filePath string, hash string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}

	defer file.Close()

	hasher := md5.New()

	_, err = io.Copy(hasher, file)
	if err != nil {
		return false
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)) == hash
}
