package readenv

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

type Env struct {
	apiKey   string
	apiUri   string
	wsUri    string
	uuid     string
	macAdr   string
	name     string
	location string
}

func ReadEnv() (*Env, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, err
	}
	macAdrDevice, err := getMacAdr()
	if err != nil {
		return nil, errors.New("FAILED TO GET MAC ADDRESS")
	}

	uuidDevice, err := getUuid()
	if err != nil {
		return nil, errors.New("FAILED TO GET UUID")
	}

	nameDevice := os.Getenv("NAME")
	locationDevice := os.Getenv("LOCATION")
	apiUri := os.Getenv("API_URI")
	wsUri := os.Getenv("WS_URI")

	// check if api key exist in .env file
	isApiKeyExist, err := checkKeyExist("API_KEY")
	if err != nil {
		return nil, errors.New("FAILED TO CHECK API KEY")
	}

	var apiKey string
	if !isApiKeyExist {
		apiKey, err = getApiKey(apiUri, macAdrDevice, uuidDevice, nameDevice, locationDevice)
		if err != nil {
			return nil, errors.New("FAILED TO GET API KEY")
		}
	} else {
		apiKey = os.Getenv("API_KEY")
	}

	return &Env{
		uuid:     uuidDevice,
		name:     nameDevice,
		location: locationDevice,
		macAdr:   macAdrDevice,
		apiUri:   apiUri,
		wsUri:    wsUri,
		apiKey:   apiKey,
	}, nil
}
