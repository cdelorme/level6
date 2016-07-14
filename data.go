package level6

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"io"
	"os"
)

var ErrorSameFile = errors.New("same file...")

func CheckCrc32(path string) (string, error) {
	hasher := crc32.New(nil)
	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()

	if _, err := io.Copy(hasher, in); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func CheckSha256(path string) (string, error) {
	hasher := sha256.New()

	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()

	if _, err := io.Copy(hasher, in); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func CheckBytes(one, two string) (bool, error) {
	if one == two {
		return true, ErrorSameFile
	}

	inOne, err := os.Open(one)
	if err != nil {
		return false, err
	}
	defer inOne.Close()
	inTwo, err := os.Open(two)
	if err != nil {
		return false, err
	}
	defer inTwo.Close()

	statOne, err := inOne.Stat()
	if err != nil {
		return false, err
	}
	statTwo, err := inTwo.Stat()
	if err != nil {
		return false, err
	}
	if os.SameFile(statOne, statTwo) {
		return false, nil
	}

	bufferOne, bufferTwo := make([]byte, 32*1024), make([]byte, 32*1024)
	readerOne, readerTwo := bufio.NewReader(inOne), bufio.NewReader(inTwo)
	for {
		_, errOne := readerOne.Read(bufferOne)
		_, errTwo := readerTwo.Read(bufferTwo)
		if errOne == io.EOF || io.EOF == errTwo {
			if errOne == errTwo {
				break
			}
			return false, nil
		}
		if bytes.Compare(bufferOne, bufferTwo) != 0 {
			return false, nil
		}
	}

	return true, nil
}

type Data struct {
	files map[int64][]string
	stats stats
	l     logger
	err   error
}

func (self *Data) Stats(s stats) {
	self.stats = s
}

func (self *Data) Logger(l logger) {
	self.l = l
}

func (self *Data) Walk(path string, f os.FileInfo, err error) error {
	if self.files == nil {
		self.files = make(map[int64][]string)
	}
	if _, ok := self.files[f.Size()]; !ok {
		self.files[f.Size()] = make([]string, 0)
	}
	self.files[f.Size()] = append(self.files[f.Size()], path)
	self.stats.Add(StatsFiles, 1)
	return err
}

func (self *Data) Execute() ([][]string, error) {
	duplicates := [][]string{}

	for size, _ := range self.files {
		if len(self.files[size]) <= 1 {
			continue
		}

		crc32hashes := map[string][]string{}
		for i, _ := range self.files[size] {
			hash, err := CheckCrc32(self.files[size][i])
			if err != nil {
				self.l.Error(err.Error())
				self.err = err
				continue
			}
			if _, ok := crc32hashes[hash]; !ok {
				crc32hashes[hash] = make([]string, 0)
			}
			crc32hashes[hash] = append(crc32hashes[hash], self.files[size][i])
			self.stats.Add(StatsHashCrc32, 1)
		}
		delete(self.files, size)

		sha256hashes := map[string][]string{}
		for _, hashes := range crc32hashes {
			if len(hashes) <= 1 {
				continue
			}
			for _, v := range hashes {
				hash, err := CheckSha256(v)
				if err != nil {
					self.l.Error(err.Error())
					self.err = err
					continue
				}

				if _, ok := sha256hashes[hash]; !ok {
					sha256hashes[hash] = make([]string, 0)
				}
				sha256hashes[hash] = append(sha256hashes[hash], v)
				self.stats.Add(StatsHashSha256, 1)
			}
		}

		for _, hashes := range sha256hashes {
			if len(hashes) <= 1 {
				continue
			}
			for i := 0; i < len(hashes); i++ {
				var matches []string
				for k := 0; k < len(hashes); k++ {
					if i == k {
						continue
					} else if m, e := CheckBytes(hashes[i], hashes[k]); e != nil {
						self.l.Error(e.Error())
					} else if m {
						matches = append(matches, hashes[k])
						hashes = append(hashes[:k], hashes[k+1:]...)
						k--
					}
				}
				if len(matches) > 0 {
					matches = append(matches, hashes[i])
					hashes = append(hashes[:i], hashes[i+1:]...)
					i--
					duplicates = append(duplicates, matches)
					self.stats.Add(StatsDuplicates, len(matches)-1)
				}
			}
		}
	}

	return duplicates, self.err
}
