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
		data := m2mapi.ResolvePoint{
			NE: m2mapi.SquarePoint{Lat: 35.531, Lon: 139.531},
			SW: m2mapi.SquarePoint{Lat: 35.53, Lon: 139.53},
		}
	*/
	/*
		data := m2mapi.ResolveNode{
			VPointID:  "11529215046068469760",
			CapsInput: []string{"MaxTemp", "MaxHumid"},
		}
	*/
	data := m2mapi.ResolvePastNode{
		VNodeID:       "9223372036854775808",
		Capability:    "MaxTemp",
		Period:        m2mapi.PeriodInput{Start: "2023-08-16 04:55:50 +0900 JST", End: "2023-08-16 04:56:00 +0900 JST"},
		SocketAddress: "192.168.1.1:11000",
	}
	client_data, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error marshalling data: ", err)
		return
	}
	response, err := http.Post("http://localhost:8080/m2mapi/data/past/node", "application/json", bytes.NewBuffer(client_data))
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
