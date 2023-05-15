package hash

type Hash struct {
	MD5  string `json:"MD5"`
	SHA1 string `json:"SHA1"`
	SHA2 SHA2   `json:"SHA2"`
	SHA3 SHA3   `json:"SHA3"`
	CRC  CRC    `json:"CRC"`
}

type SHA2 struct {
	SHA224     string `json:"SHA224"`
	SHA256     string `json:"SHA256"`
	SHA384     string `json:"SHA384"`
	SHA512     string `json:"SHA512"`
	SHA512_224 string `json:"SHA512_224"`
	SHA512_256 string `json:"SHA512_256"`
}

type SHA3 struct {
	SHA256 string `json:"SHA256"`
	SHA512 string `json:"SHA512"`
}

type CRC struct {
	CRC32 string `json:"CRC32"`
	CRC64 string `json:"CRC64"`
}
