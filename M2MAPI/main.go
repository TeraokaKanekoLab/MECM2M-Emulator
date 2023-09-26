package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/message"

	"github.com/joho/godotenv"
)

var (
	port                 string
	cloud_server_ip_port string
	ip_address           string

	ad_cache           map[string]m2mapi.AreaDescriptor = make(map[string]m2mapi.AreaDescriptor)
	area_mapping_cache []m2mapi.MECCoverArea
)

func init() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		log.Fatal(err)
	}
	port = os.Getenv("M2M_API_PORT")
	ip_address = os.Getenv("IP_ADDRESS")
	cloud_server_ip_port = os.Getenv("CLOUD_SERVER_IP_PORT")
}

func main() {
	/*
		// Mainプロセスのコマンドラインからシミュレーション実行開始シグナルを受信するまで待機
		signals_from_main := make(chan os.Signal, 1)

		// 停止しているプロセスを再開するために送信されるシグナル，SIGCONT(=18)を受信するように設定
		signal.Notify(signals_from_main, syscall.SIGCONT)

		// シグナルを待機
		fmt.Println("Waiting for signal...")
		sig := <-signals_from_main

		// 受信したシグナルを表示
		fmt.Printf("Received signal: %v\n", sig)
	*/
	http.HandleFunc("/m2mapi/area", resolveArea)
	http.HandleFunc("/m2mapi/area/extend", extendAD)
	http.HandleFunc("/m2mapi/node", resolveNode)
	http.HandleFunc("/m2mapi/data/past/node", resolvePastNode)
	http.HandleFunc("/m2mapi/data/current/node", resolveCurrentNode)
	http.HandleFunc("/m2mapi/data/condition/node", resolveConditionNode)
	http.HandleFunc("/m2mapi/data/past/area", resolvePastArea)
	http.HandleFunc("/m2mapi/data/current/area", resolveCurrentArea)
	http.HandleFunc("/m2mapi/data/condition/area", resolveConditionArea)
	http.HandleFunc("/m2mapi/actuate", actuate)
	http.HandleFunc("/hello", hello)

	log.Printf("Connect to http://%s:%s/ for M2M API", ip_address, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func hello(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello World\n")
}

func resolveArea(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveArea: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveArea{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveArea: Error missmatching packet format", http.StatusInternalServerError)
		}

		if inputFormat.TransmitFlag {
			fmt.Println("Success transmit!!")
		} else {
			// GraphDBへの問い合わせ
			results := resolveAreaFunction(inputFormat.SW, inputFormat.NE)
			results_str, err := json.Marshal(results)
			if err != nil {
				fmt.Println("Error marshaling data: ", err)
				return
			}
			fmt.Fprintf(w, "%v\n", string(results_str))
		}
	} else {
		http.Error(w, "resolveArea: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
	fmt.Println(ad_cache)
}

func extendAD(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "extendAD: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ExtendAD{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "extendAD: Error missmatching packet format", http.StatusInternalServerError)
		}

		output := m2mapi.ExtendAD{}
		if value, ok := ad_cache[inputFormat.AD]; ok {
			value.TTL.Add(1 * time.Hour)
			output.Flag = true
		} else {
			output.Flag = false
		}

		fmt.Fprintf(w, "%v\n", output)
	} else {
		http.Error(w, "extendAD: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolveNode(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// GraphDBへの問い合わせ
		results := resolveNodeFunction(inputFormat.AD, inputFormat.Capabilities, inputFormat.NodeType)
		results_str, err := json.Marshal(results)
		if err != nil {
			fmt.Println("Error marshaling data: ", err)
			return
		}

		fmt.Println(string(results_str))
		fmt.Fprintf(w, "%v\n", string(results_str))
	} else {
		http.Error(w, "resolveNode: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolvePastNode(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolvePastNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveDataByNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolvePastNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNodeへリクエスト転送
		results := resolvePastNodeFunction(inputFormat.VNodeID, inputFormat.Capability, inputFormat.SocketAddress, inputFormat.Period)

		fmt.Fprintf(w, "%v\n", results)
	} else {
		http.Error(w, "resolvePastNode: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolveCurrentNode(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveCurrentNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveDataByNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveCurrentNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNodeへリクエスト転送
		results := resolveCurrentNodeFunction(inputFormat.VNodeID, inputFormat.Capability, inputFormat.SocketAddress)

		fmt.Fprintf(w, "%v\n", results)
	} else {
		http.Error(w, "resolveCurrentNode: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolveConditionNode(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveConditionNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveDataByNode{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveConditionNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNode へリクエスト転送
		results := resolveConditionNodeFunction(inputFormat.VNodeID, inputFormat.Capability, inputFormat.SocketAddress, inputFormat.Condition)

		fmt.Fprintf(w, "%v\n", results)
	} else {
		http.Error(w, "resolveConditionNode: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolvePastArea(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolvePastArea: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveDataByArea{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolvePastArea: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNode もしくは VMNode へリクエスト転送
		results := resolvePastAreaFunction(inputFormat.AD, inputFormat.Capability, inputFormat.NodeType, inputFormat.Period)

		fmt.Fprintf(w, "%v\n", results)
	} else {
		http.Error(w, "resolvePastArea: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolveCurrentArea(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveCurrentArea: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveDataByArea{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveCurrentArea: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNode もしくは VMNode へリクエスト転送
		results := resolveCurrentAreaFunction(inputFormat.AD, inputFormat.Capability, inputFormat.NodeType)

		fmt.Fprintf(w, "%v\n", results)
	} else {
		http.Error(w, "resolveCurrentArea: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolveConditionArea(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveConditionArea: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.ResolveDataByArea{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveConditionArea: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNode もしくは VMNode へリクエスト転送
		results := resolveConditionAreaFunction(inputFormat.AD, inputFormat.Capability, inputFormat.NodeType, inputFormat.Condition)

		fmt.Fprintf(w, "%v\n", results)
	} else {
		http.Error(w, "resolveCurrentArea: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func actuate(w http.ResponseWriter, r *http.Request) {
	// POST リクエストのみを受信する
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "actuate: Error reading request body", http.StatusInternalServerError)
			return
		}
		inputFormat := &m2mapi.Actuate{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "actuate: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNode もしくは VMNode へリクエスト転送
		results := actuateFunction(inputFormat.VNodeID, inputFormat.Action, inputFormat.SocketAddress, inputFormat.Parameter)

		fmt.Fprintf(w, "%v\n", results)
	} else {
		http.Error(w, "actuate: Method not supported: Only POST request", http.StatusMethodNotAllowed)
	}
}

func resolveAreaFunction(sw, ne m2mapi.SquarePoint) m2mapi.ResolveArea {
	// Cloud Sever に聞きに行くかのフラグと，エリア解決時に用いるServerIPのスライスの定義
	ask_cloud_flag := true
	var target_mec_server []string

	// 1. area_mapping_cache の情報と入力の矩形範囲を比べて，矩形範囲が area_mapping_cache の範囲と被っているか確認．被っている area_mapping がなければ，Cloud Serverに聞きに行く
	for _, cover_area := range area_mapping_cache {
		if (ne.Lat <= cover_area.MinLat || ne.Lon <= cover_area.MinLon) || (sw.Lat >= cover_area.MaxLat || sw.Lon >= cover_area.MaxLon) {
			// 対象領域でない
			//fmt.Println("Not target: ", cover_area.ServerIP)
		} else {
			// 対象領域である
			ask_cloud_flag = false
			target_mec_server = append(target_mec_server, cover_area.ServerIP)
		}
	}

	// 2. area_mapping_cache に情報がない，もしくは area_mapping_cache に対象の情報がない場合，Cloud Serverに対象サーバを聞きに行く
	if ask_cloud_flag {
		fmt.Println("Ask Cloud Server")
		area_mapping_data_request := m2mapi.AreaMapping{
			SW: sw,
			NE: ne,
		}
		transmit_data, _ := json.Marshal(area_mapping_data_request)
		cloud_url := "http://" + cloud_server_ip_port + "/m2mapi/area/mapping"
		area_mapping_data_response, err := http.Post(cloud_url, "application/json", bytes.NewBuffer(transmit_data))
		if err != nil {
			fmt.Println("Error making request: ", err)
		}
		defer area_mapping_data_response.Body.Close()

		body, err := io.ReadAll(area_mapping_data_response.Body)
		if err != nil {
			panic(err)
		}

		var area_mapping_output []m2mapi.AreaMapping
		if err = json.Unmarshal(body, &area_mapping_output); err != nil {
			fmt.Println("Error Unmarshaling: ", err)
		}

		// area_mapping_cache にマッピング情報をキャッシュ
		for _, area_mapping := range area_mapping_output {
			area_mapping_cache = append(area_mapping_cache, area_mapping.MECCoverArea)
			target_mec_server = append(target_mec_server, area_mapping.MECCoverArea.ServerIP)
		}
	}

	//fmt.Println("target mec server: ", target_mec_server)

	// 3. 対象となったMEC ServerのLocal GraphDBに検索をかける．
	// この時，自MEC Serverに問い合わせる場合は，そのまま検索クエリを投げればいいが，他MEC Serverの場合，一度 M2M API を挟まなければいけない
	// 出力結果は，PAreaID, VNodeID, VNodeSocketAddress, VMNodeRSocketAddress
	payload := `{"statements": [{"statement": "MATCH (a:PArea)-[:contains]->(vn:VNode) WHERE a.NE[0] > ` + strconv.FormatFloat(sw.Lat, 'f', 4, 64) + ` and a.NE[1] > ` + strconv.FormatFloat(sw.Lon, 'f', 4, 64) + ` and a.SW[0] < ` + strconv.FormatFloat(ne.Lat, 'f', 4, 64) + ` and a.SW[1] < ` + strconv.FormatFloat(ne.Lon, 'f', 4, 64) + ` return a.PAreaID, vn.VNodeID, vn.SocketAddress, vn.VMNodeRSocketAddress;"}]}`
	results := m2mapi.ResolveArea{}
	area_desc := m2mapi.AreaDescriptor{}
	for _, server_ip := range target_mec_server {
		if server_ip == ip_address {
			// 自MEC ServerのLocal GraphDBへの検索
			graphdb_url := "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + server_ip + ":" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
			req, _ := http.NewRequest("POST", graphdb_url, bytes.NewBuffer([]byte(payload)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "*/*")

			client := new(http.Client)
			resp, err := client.Do(req)
			if err != nil {
				message.MyError(err, "resolvePointFunction > client.Do()")
			}
			defer resp.Body.Close()

			byteArray, _ := io.ReadAll(resp.Body)
			values := bodyGraphDB(byteArray)
			//fmt.Println(values)

			var row_data interface{}
			for _, v1 := range values {
				for k2, v2 := range v1.(map[string]interface{}) {
					if k2 == "data" {
						for _, v3 := range v2.([]interface{}) {
							for k4, v4 := range v3.(map[string]interface{}) {
								if k4 == "row" {
									row_data = v4
									dataArray := row_data.([]interface{})
									area_desc.PAreaID = addIfNotExists(area_desc.PAreaID, dataArray[0].(string))
									var vmnoder_socket_address string
									if dataArray[3] == nil {
										vmnoder_socket_address = ""
									} else {
										vmnoder_socket_address = dataArray[3].(string)
									}
									vnode_set := m2mapi.VNodeSet{VNodeID: dataArray[1].(string), VNodeSocketAddress: dataArray[2].(string), VMNodeRSocketAddress: vmnoder_socket_address}
									area_desc.VNode = append(area_desc.VNode, vnode_set)
									currentTime := time.Now()
									area_desc.TTL = currentTime.Add(1 * time.Hour)
									results.AD = fmt.Sprintf("%x", uintptr(unsafe.Pointer(&area_desc)))
									results.TTL = area_desc.TTL
								}
							}
						}
					}
				}
			}
			area_desc.ServerIP = append(area_desc.ServerIP, server_ip)
		} else {
			// 他MEC Serverへリクエスト転送
			transmit_m2mapi_url := "http://" + server_ip + ":" + os.Getenv("M2M_API_PORT") + "/m2mapi/area"
			transmit_m2mapi_data := m2mapi.ResolveArea{
				NE:           m2mapi.SquarePoint{Lat: ne.Lat, Lon: ne.Lon},
				SW:           m2mapi.SquarePoint{Lat: sw.Lat, Lon: sw.Lon},
				TransmitFlag: true,
			}
			byte_data, _ := json.Marshal(transmit_m2mapi_data)
			response, err := http.Post(transmit_m2mapi_url, "application/json", bytes.NewBuffer(byte_data))
			if err != nil {
				fmt.Println("Error making response: ", err)
				panic(err)
			}
			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			if err != nil {
				panic(err)
			}
			fmt.Println("Transmit: ", string(body))
		}
	}

	ad_cache[results.AD] = area_desc
	results.Descriptor = area_desc
	return results
}

func resolveNodeFunction(ad string, caps []string, node_type string) []m2mapi.ResolveNode {
	// エッジサーバのカバー領域をもとに，検索範囲が少しでもカバー領域から外れていればクラウドサーバのDBへ検索
	// そうでなければ，通常通りローカルのDBへ検索
	results := []m2mapi.ResolveNode{}

	area_desc := ad_cache[ad]

	var format_capabilities []string
	for _, capability := range caps {
		capability = "\\\"" + capability + "\\\""
		format_capabilities = append(format_capabilities, capability)
	}

	if node_type == "All" || node_type == "VSNode" {
		var format_vsnodes []string
		for _, vsnode := range area_desc.VNode {
			vnode_id := "\\\"" + vsnode.VNodeID + "\\\""
			format_vsnodes = append(format_vsnodes, vnode_id)
		}
		vnode_payload := `{"statements": [{"statement": "MATCH (vs:VSNode)-[:isPhysicalizedBy]->(ps:PSNode) WHERE vs.VNodeID IN [` + strings.Join(format_vsnodes, ", ") + `] and ps.Capability IN [` + strings.Join(format_capabilities, ", ") + `] return vs.VNodeID, vs.SocketAddress;"}]}`
		for _, server_ip := range area_desc.ServerIP {
			graphdb_url := "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + server_ip + ":" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
			req, _ := http.NewRequest("POST", graphdb_url, bytes.NewBuffer([]byte(vnode_payload)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "*/*")

			client := new(http.Client)
			resp, err := client.Do(req)
			if err != nil {
				message.MyError(err, "resolvePointFunction > client.Do()")
			}
			defer resp.Body.Close()

			byteArray, _ := io.ReadAll(resp.Body)
			values := bodyGraphDB(byteArray)

			var row_data interface{}
			for _, v1 := range values {
				for k2, v2 := range v1.(map[string]interface{}) {
					if k2 == "data" {
						for _, v3 := range v2.([]interface{}) {
							for k4, v4 := range v3.(map[string]interface{}) {
								if k4 == "row" {
									row_data = v4
									dataArray := row_data.([]interface{})
									result := m2mapi.ResolveNode{}
									result.VNode.VNodeID = dataArray[0].(string)
									result.VNode.VNodeSocketAddress = dataArray[1].(string)
									results = append(results, result)
								}
							}
						}
					}
				}
			}
		}
	}

	if node_type == "All" || node_type == "VMNode" {
		var format_areas []string
		for _, area := range area_desc.PAreaID {
			area = "\\\"" + area + "\\\""
			format_areas = append(format_areas, area)
		}
		vmnode_payload := `{"statements": [{"statement": "MATCH (a:PArea)-[:contains]->(vm:VMNode)-[:isPhysicalizedBy]->(pm:PMNode) WHERE a.PAreaID IN [` + strings.Join(format_areas, ", ") + `] and pm.Capability IN [` + strings.Join(format_capabilities, ", ") + `] return vm.VNodeID, vm.SocketAddress, vm.VMNodeRSocketAddress;"}]}`
		for _, server_ip := range area_desc.ServerIP {
			graphdb_url := "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + server_ip + ":" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
			req, _ := http.NewRequest("POST", graphdb_url, bytes.NewBuffer([]byte(vmnode_payload)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "*/*")

			client := new(http.Client)
			resp, err := client.Do(req)
			if err != nil {
				message.MyError(err, "resolvePointFunction > client.Do()")
			}
			defer resp.Body.Close()

			byteArray, _ := io.ReadAll(resp.Body)
			values := bodyGraphDB(byteArray)

			var row_data interface{}
			for _, v1 := range values {
				for k2, v2 := range v1.(map[string]interface{}) {
					if k2 == "data" {
						for _, v3 := range v2.([]interface{}) {
							for k4, v4 := range v3.(map[string]interface{}) {
								if k4 == "row" {
									row_data = v4
									dataArray := row_data.([]interface{})
									result := m2mapi.ResolveNode{}
									result.VNode.VNodeID = dataArray[0].(string)
									result.VNode.VNodeSocketAddress = dataArray[1].(string)
									result.VNode.VMNodeRSocketAddress = dataArray[2].(string)
									results = append(results, result)
								}
							}
						}
					}
				}
			}
		}
	}

	return results
}

func resolvePastNodeFunction(vnode_id, capability, socket_address string, period m2mapi.PeriodInput) m2mapi.ResolveDataByNode {
	null_data := m2mapi.ResolveDataByNode{VNodeID: "NULL"}

	request_data := m2mapi.ResolveDataByNode{
		VNodeID:    vnode_id,
		Capability: capability,
		Period:     m2mapi.PeriodInput{Start: period.Start, End: period.End},
	}
	transmit_data, err := json.Marshal(request_data)
	if err != nil {
		fmt.Println("Error marshalling data: ", err)
		return null_data
	}
	transmit_url := "http://" + socket_address + "/primapi/data/past/node"
	response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
	if err != nil {
		fmt.Println("Error making request:", err)
		return null_data
	}
	defer response_data.Body.Close()

	byteArray, _ := io.ReadAll(response_data.Body)
	var results m2mapi.ResolveDataByNode
	if err = json.Unmarshal(byteArray, &results); err != nil {
		fmt.Println("Error unmarshaling data: ", err)
		return null_data
	}

	return results
}

func resolveCurrentNodeFunction(vnode_id, capability, socket_address string) m2mapi.ResolveDataByNode {
	null_data := m2mapi.ResolveDataByNode{VNodeID: "NULL"}

	request_data := m2mapi.ResolveDataByNode{
		VNodeID:    vnode_id,
		Capability: capability,
	}
	transmit_data, err := json.Marshal(request_data)
	if err != nil {
		fmt.Println("Error marshalling data: ", err)
		return null_data
	}
	transmit_url := "http://" + socket_address + "/primapi/data/current/node"
	response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
	if err != nil {
		fmt.Println("Error making request: ", err)
		return null_data
	}
	defer response_data.Body.Close()

	byteArray, _ := io.ReadAll(response_data.Body)
	var results m2mapi.ResolveDataByNode
	if err = json.Unmarshal(byteArray, &results); err != nil {
		fmt.Println("Error unmarshaling data: ", err)
		return null_data
	}

	return results
}

func resolveConditionNodeFunction(vnode_id, capability, socket_address string, condition m2mapi.ConditionInput) m2mapi.ResolveDataByNode {
	null_data := m2mapi.ResolveDataByNode{VNodeID: "NULL"}

	request_data := m2mapi.ResolveDataByNode{
		VNodeID:    vnode_id,
		Capability: capability,
		Condition:  condition,
	}
	transmit_data, err := json.Marshal(request_data)
	if err != nil {
		fmt.Println("Error marshaling data: ", err)
		return null_data
	}
	transmit_url := "http://" + socket_address + "/primapi/data/condition/node"
	response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
	if err != nil {
		fmt.Println("Error making request: ", err)
		return null_data
	}
	defer response_data.Body.Close()

	byteArray, _ := io.ReadAll(response_data.Body)
	var results m2mapi.ResolveDataByNode
	if err = json.Unmarshal(byteArray, &results); err != nil {
		fmt.Println("Error unmarshaling data: ", err)
		return null_data
	}

	return results
}

func resolvePastAreaFunction(ad, capability, node_type string, period m2mapi.PeriodInput) m2mapi.ResolveDataByArea {
	null_data := m2mapi.ResolveDataByArea{AD: "NULL"}
	var results m2mapi.ResolveDataByArea

	// ADに含まれるすべてのVNodeIDに対して過去データ取得リクエストを転送したい．
	area_desc := ad_cache[ad]
	if node_type == "All" || node_type == "VSNode" {
		for _, vsnode := range area_desc.VNode {
			request_data := m2mapi.ResolveDataByNode{
				VNodeID:    vsnode.VNodeID,
				Capability: capability,
				Period:     m2mapi.PeriodInput{Start: period.Start, End: period.End},
			}

			transmit_data, err := json.Marshal(request_data)
			if err != nil {
				fmt.Println("Error marshaling data: ", err)
				return null_data
			}
			transmit_url := "http://" + vsnode.VNodeSocketAddress + "/primapi/data/past/node"
			response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
			if err != nil {
				fmt.Println("Error making request: ", err)
				return null_data
			}
			defer response_data.Body.Close()

			byteArray, _ := io.ReadAll(response_data.Body)
			var result m2mapi.ResolveDataByNode
			if err := json.Unmarshal(byteArray, &result); err != nil {
				fmt.Println("Error unmarshaling data: ", err)
				return null_data
			}

			data := m2mapi.SensorData{
				VNodeID: result.VNodeID,
				Values:  result.Values,
			}
			results.Datas = append(results.Datas, data)
		}
	}

	if node_type == "All" || node_type == "VMNode" {
		// はじめに，ADに登録されているPAreaIDに存在するPMNodeとそのソケットアドレスを検索する
		vmnode_results_by_resolve_node := resolveNodeFunction(ad, []string{capability}, node_type)
		for _, vmnode_result := range vmnode_results_by_resolve_node {
			request_data := m2mapi.ResolveDataByNode{
				VNodeID:    vmnode_result.VNode.VNodeID,
				Capability: capability,
				Period:     m2mapi.PeriodInput{Start: period.Start, End: period.End},
			}

			transmit_data, err := json.Marshal(request_data)
			if err != nil {
				fmt.Println("Error marhsaling data: ", err)
				return null_data
			}
			transmit_url := "http://" + vmnode_result.VNode.VNodeSocketAddress + "/primpai/data/past/node"
			response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
			if err != nil {
				fmt.Println("Error making request: ", err)
				return null_data
			}
			defer response_data.Body.Close()

			byteArray, _ := io.ReadAll(response_data.Body)
			var result m2mapi.ResolveDataByNode
			if err := json.Unmarshal(byteArray, &result); err != nil {
				fmt.Println("Error unmarshaling data: ", err)
				return null_data
			}

			data := m2mapi.SensorData{
				VNodeID: result.VNodeID,
				Values:  result.Values,
			}
			results.Datas = append(results.Datas, data)
		}
	}

	return results
}

func resolveCurrentAreaFunction(ad, capability, node_type string) m2mapi.ResolveDataByArea {
	null_data := m2mapi.ResolveDataByArea{AD: "NULL"}
	var results m2mapi.ResolveDataByArea

	// ADに含まれるすべてのVNodeIDに対して現在データ取得リクエストを転送したい．
	if node_type == "All" || node_type == "VSNode" {
		// はじめに，ADに登録されているVSNodeのうち，指定したCapabilityを持つものだけを抽出する
		vsnode_results_by_resolve_node := resolveNodeFunction(ad, []string{capability}, node_type)
		for _, vsnode_result := range vsnode_results_by_resolve_node {
			request_data := m2mapi.ResolveDataByNode{
				VNodeID:    vsnode_result.VNode.VNodeID,
				Capability: capability,
			}

			transmit_data, err := json.Marshal(request_data)
			if err != nil {
				fmt.Println("Error marshaling data: ", err)
				return null_data
			}
			// VSNodeへ転送
			transmit_url := "http://" + vsnode_result.VNode.VNodeSocketAddress + "/primapi/data/current/node"
			response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
			if err != nil {
				fmt.Println("Error making request: ", err)
				return null_data
			}
			defer response_data.Body.Close()

			byteArray, _ := io.ReadAll(response_data.Body)
			var result m2mapi.ResolveDataByNode
			if err := json.Unmarshal(byteArray, &result); err != nil {
				fmt.Println("Error unmarshaling data: ", err)
				return null_data
			}

			data := m2mapi.SensorData{
				VNodeID: result.VNodeID,
				Values:  result.Values,
			}
			results.Datas = append(results.Datas, data)
		}
	}

	if node_type == "All" || node_type == "VMNode" {
		// はじめに，ADに登録されているPAreaIDに存在するPMNodeとそのソケットアドレスを検索する
		vmnode_results_by_resolve_node := resolveNodeFunction(ad, []string{capability}, node_type)
		for _, vmnode_result := range vmnode_results_by_resolve_node {
			request_data := m2mapi.ResolveDataByNode{
				VNodeID:    vmnode_result.VNode.VNodeID,
				Capability: capability,
			}

			transmit_data, err := json.Marshal(request_data)
			if err != nil {
				fmt.Println("Error marhsaling data: ", err)
				return null_data
			}
			// VSNodeへ転送
			transmit_url := "http://" + vmnode_result.VNode.VMNodeRSocketAddress + "/primpai/data/current/node"
			response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
			if err != nil {
				fmt.Println("Error making request: ", err)
				return null_data
			}
			defer response_data.Body.Close()

			byteArray, _ := io.ReadAll(response_data.Body)
			var result m2mapi.ResolveDataByNode
			if err := json.Unmarshal(byteArray, &result); err != nil {
				fmt.Println("Error unmarshaling data: ", err)
				return null_data
			}

			data := m2mapi.SensorData{
				VNodeID: result.VNodeID,
				Values:  result.Values,
			}
			results.Datas = append(results.Datas, data)
		}
	}

	return results
}

func resolveConditionAreaFunction(ad, capability, node_type string, condition m2mapi.ConditionInput) m2mapi.ResolveDataByArea {
	null_data := m2mapi.ResolveDataByArea{AD: "NULL"}
	var results m2mapi.ResolveDataByArea

	// ADに含まれるすべてのVNodeIDに対して現在データ取得リクエストを転送したい．
	if node_type == "All" || node_type == "VSNode" {
		// はじめに，ADに登録されているVSNodeのうち，指定したCapabilityを持つものだけを抽出する
		vsnode_results_by_resolve_node := resolveNodeFunction(ad, []string{capability}, node_type)
		for _, vsnode_result := range vsnode_results_by_resolve_node {
			request_data := m2mapi.ResolveDataByNode{
				VNodeID:    vsnode_result.VNode.VNodeID,
				Capability: capability,
				Condition:  condition,
			}

			transmit_data, err := json.Marshal(request_data)
			if err != nil {
				fmt.Println("Error marshaling data: ", err)
				return null_data
			}
			// VSNodeへ転送
			transmit_url := "http://" + vsnode_result.VNode.VNodeSocketAddress + "/primapi/data/condition/node"
			response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
			if err != nil {
				fmt.Println("Error making request: ", err)
				return null_data
			}
			defer response_data.Body.Close()

			byteArray, _ := io.ReadAll(response_data.Body)
			var result m2mapi.ResolveDataByNode
			if err := json.Unmarshal(byteArray, &result); err != nil {
				fmt.Println("Error unmarshaling data: ", err)
				return null_data
			}

			data := m2mapi.SensorData{
				VNodeID: result.VNodeID,
				Values:  result.Values,
			}
			results.Datas = append(results.Datas, data)
		}
	}

	if node_type == "All" || node_type == "VMNode" {
		// はじめに，ADに登録されているPAreaIDに存在するPMNodeとそのソケットアドレスを検索する
		vmnode_results_by_resolve_node := resolveNodeFunction(ad, []string{capability}, node_type)
		for _, vmnode_result := range vmnode_results_by_resolve_node {
			request_data := m2mapi.ResolveDataByNode{
				VNodeID:    vmnode_result.VNode.VNodeID,
				Capability: capability,
				Condition:  condition,
			}

			transmit_data, err := json.Marshal(request_data)
			if err != nil {
				fmt.Println("Error marhsaling data: ", err)
				return null_data
			}
			// VMNodeRへ転送
			transmit_url := "http://" + vmnode_result.VNode.VMNodeRSocketAddress + "/primpai/data/condition/node"
			response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
			if err != nil {
				fmt.Println("Error making request: ", err)
				return null_data
			}
			defer response_data.Body.Close()

			byteArray, _ := io.ReadAll(response_data.Body)
			var result m2mapi.ResolveDataByNode
			if err := json.Unmarshal(byteArray, &result); err != nil {
				fmt.Println("Error unmarshaling data: ", err)
				return null_data
			}

			data := m2mapi.SensorData{
				VNodeID: result.VNodeID,
				Values:  result.Values,
			}
			results.Datas = append(results.Datas, data)
		}
	}

	return results
}

func actuateFunction(vnode_id, action, socket_address string, parameter float64) m2mapi.Actuate {
	null_data := m2mapi.Actuate{VNodeID: "NULL"}

	request_data := m2mapi.Actuate{
		VNodeID:   vnode_id,
		Action:    action,
		Parameter: parameter,
	}
	transmit_data, err := json.Marshal(request_data)
	if err != nil {
		fmt.Println("Error marshaling data: ", err)
		return null_data
	}
	transmit_url := "http://" + socket_address + "/primapi/actuate"
	response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
	if err != nil {
		fmt.Println("Error making request: ", err)
		return null_data
	}
	defer response_data.Body.Close()

	byteArray, _ := io.ReadAll(response_data.Body)
	var results m2mapi.Actuate
	if err = json.Unmarshal(byteArray, &results); err != nil {
		fmt.Println("Error unmarshaling data: ", err)
		return null_data
	}

	return results
}

func bodyGraphDB(byteArray []byte) []interface{} {
	var jsonBody map[string]interface{}
	if err := json.Unmarshal(byteArray, &jsonBody); err != nil {
		message.MyError(err, "bodyGraphDB > json.Unmarshal")
		return nil
	}
	var values []interface{}
	for _, v1 := range jsonBody {
		switch v1.(type) {
		case []interface{}:
			for range v1.([]interface{}) {
				values = v1.([]interface{})
			}
		case map[string]interface{}:
			for _, v2 := range v1.(map[string]interface{}) {
				switch v2.(type) {
				case []interface{}:
					values = v2.([]interface{})
				default:
				}
			}
		default:
			fmt.Println("Format Assertion False")
		}
	}
	return values
}

func addIfNotExists(slice []string, item string) []string {
	for _, v := range slice {
		if v == item {
			return slice
		}
	}
	return append(slice, item)
}
