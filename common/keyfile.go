package common

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/crypto/sha3"
)

var Verbose bool

type Keyfile struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Address string `json:"address"`
	Crypto  struct {
		Ciphertext   string `json:"ciphertext"`
		Cipherparams struct {
			Iv string `json:"iv"`
		} `json:"cipherparams"`
		Cipher          string          `json:"cipher"`
		Kdf             string          `json:"kdf"`
		KdfparamsPack   json.RawMessage `json:"kdfparams,omitempty"`
		KdfScryptParams KdfScryptparams `json:"-"`
		KdfPbkdf2params KdfPbkdf2params `json:"-"`
		Mac             string          `json:"mac"`
	} `json:"crypto"`
	Plaintext []byte `json:"-"`
	PubKey    string `json:"-"`
}

type KdfScryptparams struct {
	Dklen int    `json:"dklen"`
	Salt  string `json:"salt"`
	N     int    `json:"n"`
	R     int    `json:"r"`
	P     int    `json:"p"`
}

type KdfPbkdf2params struct {
	C     int    `json:"c"`
	Dklen int    `json:"dklen"`
	Prf   string `json:"prf"`
	Salt  string `json:"salt"`
}

//Reads and parses a json from a file
func ReadKeyfile(filename string) (*Keyfile, error) {
	filebytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return ProcessJsonBytes(filebytes)

}

//Recoveres the encryption key from password
func KeyFromPassScrypt(password []byte, params KdfScryptparams) ([]byte, error) {
	salt, err := hex.DecodeString(params.Salt)
	if err != nil {
		return nil, err
	}
	return scrypt.Key(password, salt, params.N, params.R, params.P, params.Dklen)
}

//Recoveres the encryption key from password
func KeyFromPassPbkdf2(password []byte, params KdfPbkdf2params) ([]byte, error) {
	salt, err := hex.DecodeString(params.Salt)
	if err != nil {
		return nil, err
	}
	return pbkdf2.Key(password, salt, params.C, params.Dklen, sha256.New), nil

}

//Just a convenience wrapper copied from geth
func Keccak256(data ...[]byte) []byte {
	d := sha3.NewLegacyKeccak256()
	for _, b := range data {
		d.Write(b)
	}
	return d.Sum(nil)
}

func GenerateKeyFileStruct(pass []byte, kdf string, encalg string, privhex string, vanity string, caseSensitive bool, timeout int) (kf *Keyfile, err error, tries int, span time.Duration) {
	kf = &Keyfile{}

	kf.Crypto.Kdf = kdf
	kf.Crypto.Cipher = strings.ToLower(encalg)
	xuuid, err := uuid.NewUUID()
	kf.ID = xuuid.String()

	ethkey := make([]byte, 32)
	if len(privhex) > 1 {
		if privhex[:2] == "0x" {
			privhex = privhex[2:]
		}
		var privb []byte
		privb, err = hex.DecodeString(privhex)
		if err != nil {
			return
		}
		if len(privb) > 32 {
			privb = privb[:32]
		}
		copy(ethkey[32-len(privb):], privb) //padding
	} else {
		//Generate the Koblitz private key
		ethkey, err, tries, span = TimeConstraindedVanityKey(vanity, caseSensitive, timeout)
		if err != nil {
			return
		}
	}

	err = EncryptAES(kf, ethkey, pass)
	if err != nil {
		return
	}

	pubkeyeth := Scalar2Pub(ethkey)
	addr := CRCAddressFromPub(pubkeyeth)
	kf.PubKey = hex.EncodeToString(pubkeyeth)
	kf.Address = addr

	return
}

func (keyfile *Keyfile) VerifyMAC(key []byte) error {
	citx, err := hex.DecodeString(keyfile.Crypto.Ciphertext)
	if err != nil {
		return err
	}

	//verify mac
	mymac := hex.EncodeToString(Keccak256(append(key[16:32], citx...)))

	if mymac != keyfile.Crypto.Mac {
		return fmt.Errorf("MAC verification failed")
	}
	return nil
}
