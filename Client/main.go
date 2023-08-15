package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"mecm2m-Emulator/pkg/m2mapi"
)

func main() {
	/*
		point := m2mapi.ResolvePoint{
			NE: m2mapi.SquarePoint{Lat: 35.531, Lon: 139.531},
			SW: m2mapi.SquarePoint{Lat: 35.53, Lon: 139.53},
		}
	*/

	node := m2mapi.ResolveNode{
		VPointID:  "11529215046068469760",
		CapsInput: []string{"MaxTemp", "MaxHumid"},
	}

	data, err := json.Marshal(node)
	if err != nil {
		fmt.Println("Error marshalling data: ", err)
		return
	}
	response, err := http.Post("http://localhost:8080/node", "application/json", bytes.NewBuffer(data))
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}

	fmt.Println("Server Response:", string(body))
}
