package readenv

import (
	"errors"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Env struct {
	ApiKey    string
	ApiUri    string
	WsUri     string
	Uuid      string
	Name      string
	Location  string
	VideoPath string
}

func ReadEnv() (*Env, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, err
	}

	uuidDevice, err := getUuid()
	if err != nil {
		return nil, errors.New("FAILED TO GET UUID")
	}

	nameDevice := os.Getenv("NAME")
	locationDevice := os.Getenv("LOCATION")
	apiUri := os.Getenv("API_URI")
	wsUri := os.Getenv("WS_URI")
	videoPath := os.Getenv("VIDEO_PATH")
	// check if api key exist in .env file
	isApiKeyExist, err := checkKeyExist("API_KEY")
	if err != nil {
		return nil, errors.New("FAILED TO CHECK API KEY")
	}

	var apiKey string
	if !isApiKeyExist {
		apiKey, err = getApiKey(apiUri, uuidDevice, nameDevice, locationDevice)
		if err != nil {
			return nil, errors.New("FAILED TO GET API KEY")
		}
		log.Println(apiKey)
	} else {
		apiKey = os.Getenv("API_KEY")
		isValid, err := checkApiKeyValid(apiKey)
		if err != nil {
			return nil, errors.New("CANNOT VERIFY API KEY")
		}
		if !isValid {
			return nil, errors.New("API KEY IS NOT VALID")
		}
	}

	env := Env{
		Uuid:      uuidDevice,
		Name:      nameDevice,
		Location:  locationDevice,
		ApiUri:    apiUri,
		WsUri:     wsUri,
		VideoPath: videoPath,
		ApiKey:    apiKey,
	}
	err = env.Save()
	if err != nil {
		return nil, errors.New("FAILED TO SAVE .ENV FILE")
	}
	return &env, nil
}

func (env Env) toMap() map[string]string {
	envMap := make(map[string]string)
	envMap["API_KEY"] = env.ApiKey
	envMap["API_URI"] = env.ApiUri
	envMap["WS_URI"] = env.WsUri
	envMap["UUID"] = env.Uuid
	envMap["NAME"] = env.Name
	envMap["LOCATION"] = env.Location
	envMap["VIDEO_PATH"] = env.VideoPath
	return envMap
}

func (env Env) Save() error {
	envMap := env.toMap()
	err := godotenv.Write(envMap, ".env")
	if err != nil {
		return err
	}
	return nil
}
