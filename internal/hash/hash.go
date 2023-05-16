package hash

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash/crc32"
	"hash/crc64"
	"io"
	"os"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/blake2s"
	"golang.org/x/crypto/sha3"
)

const (
	B  = 1 << (10 * iota)
	KB = 1 << 10
	MB = 1 << 20
	GB = 1 << 30
	TB = 1 << 40
)

func HashFile(file *os.File) (Hash, error) {
	fileStats, err := file.Stat()
	if err != nil {
		return Hash{}, err
	}

	// If file is bigger than 100MB, don't hash it
	if fileStats.Size() > int64(100*MB) {
		return Hash{}, nil
	}

	// MD5
	hasherMD5 := md5.New()

	// SHA1
	hasherSHA1 := sha1.New()

	// SHA2
	hasherSHA244 := sha256.New224()
	hasherSHA256 := sha256.New()
	hasherSHA384 := sha512.New384()
	hasherSHA512 := sha512.New()
	hasherSHA512_224 := sha512.New512_224()
	hasherSHA512_256 := sha512.New512_256()

	// SHA3
	hasherSHA3_256 := sha3.New256()
	hasherSHA3_512 := sha3.New512()

	// CRC
	crc32Hasher := crc32.NewIEEE()
	crc64Hasher := crc64.New(crc64.MakeTable(crc64.ECMA))

	// Blake
	// TODO: Maybe add error handling. Not sure yet
	hasherBlake2b_256, _ := blake2b.New256(nil)
	hasherBlake2b_384, _ := blake2b.New384(nil)
	hasherBlake2b_512, _ := blake2b.New512(nil)
	hasherBlake2s_256, _ := blake2s.New256(nil)

	multiWriter := io.MultiWriter(
		hasherMD5,
		hasherSHA1,
		hasherSHA244,
		hasherSHA256,
		hasherSHA384,
		hasherSHA512,
		hasherSHA512_224,
		hasherSHA512_256,
		hasherSHA3_256,
		hasherSHA3_512,
		crc32Hasher,
		crc64Hasher,
		hasherBlake2b_256,
		hasherBlake2b_384,
		hasherBlake2b_512,
		hasherBlake2s_256,
	)

	_, err = io.Copy(multiWriter, file)
	if err != nil {
		return Hash{}, err
	}

	hash := Hash{
		MD5:  fmt.Sprintf("%x", hasherMD5.Sum(nil)),
		SHA1: fmt.Sprintf("%x", hasherSHA1.Sum(nil)),
		SHA2: SHA2{
			SHA224:     fmt.Sprintf("%x", hasherSHA244.Sum(nil)),
			SHA256:     fmt.Sprintf("%x", hasherSHA256.Sum(nil)),
			SHA384:     fmt.Sprintf("%x", hasherSHA384.Sum(nil)),
			SHA512:     fmt.Sprintf("%x", hasherSHA512.Sum(nil)),
			SHA512_224: fmt.Sprintf("%x", hasherSHA512_224.Sum(nil)),
			SHA512_256: fmt.Sprintf("%x", hasherSHA512_256.Sum(nil)),
		},
		SHA3: SHA3{
			SHA256: fmt.Sprintf("%x", hasherSHA3_256.Sum(nil)),
			SHA512: fmt.Sprintf("%x", hasherSHA3_512.Sum(nil)),
		},
		CRC: CRC{
			CRC32: fmt.Sprintf("%x", crc32Hasher.Sum(nil)),
			CRC64: fmt.Sprintf("%x", crc64Hasher.Sum(nil)),
		},
		Blake: Blake{
			Blake2b: Blake2b{
				Blake256: fmt.Sprintf("%x", hasherBlake2b_256.Sum(nil)),
				Blake384: fmt.Sprintf("%x", hasherBlake2b_384.Sum(nil)),
				Blake512: fmt.Sprintf("%x", hasherBlake2b_512.Sum(nil)),
			},
			Blake2s: Blake2s{
				Blake256: fmt.Sprintf("%x", hasherBlake2s_256.Sum(nil)),
			},
		},
	}

	return hash, nil
}

func Checksum(filePath string, hash string) bool {
	file, err := os.Open(filePath)

	if err != nil {
		fmt.Println(err)
		return false
	}

	defer file.Close()

	hasher := md5.New()

	_, err = io.Copy(hasher, file)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)) == hash
}
