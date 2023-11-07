package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

// Give the information of the camera to the server and received the unique API Key
func RequestAPIKey() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	exist, _ := CheckKeyIfExist("API_KEY")
	if !exist {
		// in case api key is not exist because is the first time exec
		// find Mac Addr
		macAddr := getMacAddress()
		err := addKeyIntoEnv("MAC_ADDR", macAddr)
		if err != nil {
			log.Println(err)
			return
		}

		// prepare request payload
		type Payload struct {
			MacAddr  string `json:"macAddr"`
			Name     string `json:"name"`
			Location string `json:"location"`
		}

		payload := Payload{
			MacAddr:  macAddr,
			Name:     os.Getenv("NAME"),
			Location: os.Getenv("LOCATION"),
		}
		data, err := sendPostRequest(os.Getenv("API_URI")+"/camera/initCamera", payload)
		if err != nil {
			log.Println(err)
			return
		}
		log.Println("API Key: ", data["apiKey"].(string))
		err = addKeyIntoEnv("API_KEY", data["apiKey"].(string))
	} 
}

// check if the given key is exist in the .env file
func CheckKeyIfExist(key string) (bool, error) {
	err := godotenv.Load()
	if err != nil {
		return false, err
	}

	envMap, err := godotenv.Read()
	if err != nil {
		return false, err
	}

	_, exists := envMap[key]
	return exists, nil
}

// add the new key and value into .env file
func addKeyIntoEnv(key, value string) error {
	err := godotenv.Load()
	if err != nil {
		return err
	}

	envMap, err := godotenv.Read()
	if err != nil {
		return err
	}

	envMap[key] = value
	err = godotenv.Write(envMap, ".env")
	if err != nil {
		return err
	}

	return nil
}

// get the mac address of the device
func getMacAddress() string {
	ifas, err := net.Interfaces()
	if err != nil {
		log.Println(err)
		return ""
	}
	var as []string
	for _, ifa := range ifas {
		a := ifa.HardwareAddr.String()
		if a != "" {
			as = append(as, a)
		}
	}
	macAddr := getMostRepeat(as)
	return macAddr
}

// return the most repeate element in the slice
func getMostRepeat[T comparable](slice []T) T {
	hashMap := make(map[T]int)
	var mostRepeat T
	maxCount := 0
	for _, t := range slice {
		hashMap[t]++
		if hashMap[t] > maxCount {
			mostRepeat = t
			maxCount = hashMap[t]
		}
	}
	return mostRepeat
}

func sendPostRequest(url string, data interface{}) (map[string]interface{}, error) {
	jsonValue, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	response, err := http.Post(url, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		return nil, errors.New(response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
