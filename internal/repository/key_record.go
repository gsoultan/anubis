package repository

import "time"

type KeyRecord struct {
	Kid           string
	Alg           string
	Status        string
	Purpose       string
	PublicKey     []byte
	PrivateKeyEnc []byte
	NotBefore     time.Time
	NotAfter      time.Time
	CreatedAt     time.Time
	RetiredAt     *time.Time
}
