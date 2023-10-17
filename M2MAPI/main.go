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
	"sync"
	"time"
	"unsafe"

	"mecm2m-Emulator/pkg/m2mapi"
	"mecm2m-Emulator/pkg/m2mapp"
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
	minlat_float, _ := strconv.ParseFloat(os.Getenv("MIN_LAT"), 64)
	maxlat_float, _ := strconv.ParseFloat(os.Getenv("MAX_LAT"), 64)
	minlon_float, _ := strconv.ParseFloat(os.Getenv("MIN_LON"), 64)
	maxlon_float, _ := strconv.ParseFloat(os.Getenv("MAX_LON"), 64)
	own_cover_area := m2mapi.MECCoverArea{
		ServerIP: ip_address,
		MinLat:   minlat_float,
		MaxLat:   maxlat_float,
		MinLon:   minlon_float,
		MaxLon:   maxlon_float,
	}
	area_mapping_cache = append(area_mapping_cache, own_cover_area)
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

	log.Printf("Connect to http://%s:%s/ for M2M API", ip_address, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func resolveArea(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveArea: Error reading request body", http.StatusInternalServerError)
			return
		}
		var inputFormat map[string]interface{}
		if err := json.Unmarshal(body, &inputFormat); err != nil {
			http.Error(w, "resolveArea: Error missmatching packet format", http.StatusInternalServerError)
		}

		transmit_flag := false
		pmnode_flag := false
		var app_sw, app_ne m2mapp.SquarePoint
		var trans_sw, trans_ne m2mapi.SquarePoint
		for key, value := range inputFormat {
			if key == "transmit-flag" && value.(bool) {
				// 転送経路へ
				fmt.Println("find transmit flag")
				transmit_flag = true
			} else if key == "pmnode-flag" && value.(bool) {
				// PMNode M2M APIからの転送
				fmt.Println("find pmnode flag")
				pmnode_flag = true
			} else {
				switch key {
				case "sw":
					value_map := value.(map[string]interface{})
					var swlat, swlon float64
					for key2, value2 := range value_map {
						switch key2 {
						case "Lat", "lat":
							swlat = value2.(float64)
						case "Lon", "lon":
							swlon = value2.(float64)
						}
					}
					app_sw = m2mapp.SquarePoint{Lat: swlat, Lon: swlon}
					trans_sw = m2mapi.SquarePoint{Lat: swlat, Lon: swlon}
				case "ne":
					value_map := value.(map[string]interface{})
					var nelat, nelon float64
					for key2, value2 := range value_map {
						switch key2 {
						case "Lat", "lat":
							nelat = value2.(float64)
						case "Lon", "lon":
							nelon = value2.(float64)
						}
					}
					app_ne = m2mapp.SquarePoint{Lat: nelat, Lon: nelon}
					trans_ne = m2mapi.SquarePoint{Lat: nelat, Lon: nelon}
				}
			}
		}

		if transmit_flag {
			results := resolveAreaTransmitFunction(trans_sw, trans_ne)
			results_str, err := json.Marshal(results)
			if err != nil {
				fmt.Println("Error marshaling data: ", err)
				return
			}
			fmt.Fprintf(w, "%v\n", string(results_str))
		}
		if pmnode_flag {
			// PMNode M2M API からのリクエスト
			m2mapi_results := resolveAreaFunction(app_sw, app_ne)
			// m2mapp用に成型
			results := m2mapp.ResolveAreaOutput{}
			ad := fmt.Sprintf("%x", uintptr(unsafe.Pointer(&m2mapi_results.AreaDescriptor)))
			ttl := time.Now().Add(1 * time.Hour)
			results.AD = ad
			results.TTL = ttl
			results.Descriptor = m2mapi_results.AreaDescriptor
			ad_cache[ad] = m2mapi_results.AreaDescriptor

			results_str, err := json.Marshal(results)
			if err != nil {
				fmt.Println("Error marshaling data: ", err)
				return
			}
			fmt.Fprintf(w, "%v\n", string(results_str))
		}
		if !transmit_flag && !pmnode_flag {
			// M2M App からの直接リクエスト
			m2mapi_results := resolveAreaFunction(app_sw, app_ne)
			// m2mapp用に成型
			results := m2mapp.ResolveAreaOutput{}
			ad := fmt.Sprintf("%x", uintptr(unsafe.Pointer(&m2mapi_results.AreaDescriptor)))
			ttl := time.Now().Add(1 * time.Hour)
			results.AD = ad
			results.TTL = ttl
			ad_cache[ad] = m2mapi_results.AreaDescriptor

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
			for _, ad_detail := range value.AreaDescriptorDetail {
				ad_detail.TTL.Add(1 * time.Hour)
			}
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
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "resolveNode: Error reading request body", http.StatusInternalServerError)
			return
		}
		var inputFormat map[string]interface{}
		if err := json.Unmarshal(body, &inputFormat); err != nil {
			http.Error(w, "resolveNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		transmit_flag := false
		pmnode_flag := false
		var app_ad, app_node_type, trans_node_type string
		var app_capability, trans_capability []string
		var trans_ad_detail map[string]m2mapi.AreaDescriptorDetail
		trans_ad_detail = make(map[string]m2mapi.AreaDescriptorDetail)
		var trans_ad_detail_value m2mapi.AreaDescriptorDetail
		for key, value := range inputFormat {
			if key == "transmit-flag" && value.(bool) {
				fmt.Println("finad transmit flag")
				transmit_flag = true
			} else if key == "pmnode-flag" && value.(bool) {
				fmt.Println("find pmnode flag")
				pmnode_flag = true
			} else {
				switch key {
				case "ad":
					app_ad = value.(string)
				case "capability":
					for _, v := range value.([]interface{}) {
						app_capability = append(app_capability, v.(string))
						trans_capability = append(trans_capability, v.(string))
					}
				case "node-type":
					app_node_type = value.(string)
					trans_node_type = value.(string)
				case "area-descriptor-detail":
					ad_detail := value.(map[string]interface{})
					for k2, v2 := range ad_detail {
						vv2 := v2.(map[string]interface{})
						//trans_ad_detail[k2] = v2.(m2mapi.AreaDescriptorDetail)
						for k3, v3 := range vv2 {
							switch k3 {
							case "parea-id":
								for _, parea_id := range v3.([]interface{}) {
									trans_ad_detail_value.PAreaID = append(trans_ad_detail_value.PAreaID, parea_id.(string))
								}
							case "vnode":
								vv3 := v3.([]interface{})
								var vnode_set m2mapi.VNodeSet
								for _, v4 := range vv3 {
									vv4 := v4.(map[string]interface{})
									for k5, v5 := range vv4 {
										switch k5 {
										case "vnode-id":
											vnode_set.VNodeID = v5.(string)
										case "vnode-socket-address":
											vnode_set.VNodeSocketAddress = v5.(string)
										case "vmnoder-socket-address":
											vnode_set.VMNodeRSocketAddress = v5.(string)
										}
										trans_ad_detail_value.VNode = append(trans_ad_detail_value.VNode, vnode_set)
									}
								}
							}
						}
						trans_ad_detail[k2] = trans_ad_detail_value
					}
				}
			}
		}

		if transmit_flag {
			fmt.Println("transmit_flag. trans_ad_detail: ", trans_ad_detail)
			results := resolveNodeTransmitFunction(trans_ad_detail, trans_capability, trans_node_type)
			resutls_str, err := json.Marshal(results)
			if err != nil {
				fmt.Println("Error marshaling data: ", err)
				return
			}
			fmt.Fprintf(w, "%v\n", string(resutls_str))
		}
		if pmnode_flag {
			fmt.Println("pmnode_flag. transmit_ad_detail: ", trans_ad_detail)
			results := resolveNodePMNodeFunction(trans_ad_detail, trans_capability, trans_node_type)
			resutls_str, err := json.Marshal(results)
			if err != nil {
				fmt.Println("Error marshaling data: ", err)
				return
			}
			fmt.Fprintf(w, "%v\n", string(resutls_str))
		}
		if !transmit_flag && !pmnode_flag {
			fmt.Println("no flag")
			m2mapi_results := resolveNodeFunction(app_ad, app_capability, app_node_type)
			// m2mapp用に成型
			results := m2mapp.ResolveNodeOutput{}
			results.VNode = m2mapi_results.VNode

			results_str, err := json.Marshal(results)
			if err != nil {
				fmt.Println("Error marshaling data: ", err)
				return
			}
			fmt.Fprintf(w, "%v\n", string(results_str))
		}
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
		inputFormat := &m2mapp.ResolveDataByNodeInput{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolvePastNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNodeへリクエスト転送
		m2mapi_results := resolvePastNodeFunction(inputFormat.VNodeID, inputFormat.SocketAddress, inputFormat.Capability, inputFormat.Period)
		// m2mapp用に成型
		results := m2mapp.ResolveDataByNodeOutput{}
		results.VNodeID = m2mapi_results.VNodeID
		for _, val := range m2mapi_results.Values {
			v := m2mapp.Value{
				Capability: val.Capability,
				Time:       val.Time,
				Value:      val.Value,
			}
			results.Values = append(results.Values, v)
		}

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
		inputFormat := &m2mapp.ResolveDataByNodeInput{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveCurrentNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNodeへリクエスト転送
		m2mapi_results := resolveCurrentNodeFunction(inputFormat.VNodeID, inputFormat.SocketAddress, inputFormat.Capability)
		// m2mapp用に成型
		results := m2mapp.ResolveDataByNodeOutput{}
		results.VNodeID = m2mapi_results.VNodeID
		for _, val := range m2mapi_results.Values {
			v := m2mapp.Value{
				Capability: val.Capability,
				Time:       val.Time,
				Value:      val.Value,
			}
			results.Values = append(results.Values, v)
		}

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
		inputFormat := &m2mapp.ResolveDataByNodeInput{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveConditionNode: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNode へリクエスト転送
		m2mapi_results := resolveConditionNodeFunction(inputFormat.VNodeID, inputFormat.SocketAddress, inputFormat.Capability, inputFormat.Condition)
		// m2mapi用に成型
		results := m2mapp.ResolveDataByNodeOutput{}
		results.VNodeID = m2mapi_results.VNodeID
		for _, val := range m2mapi_results.Values {
			v := m2mapp.Value{
				Capability: val.Capability,
				Time:       val.Time,
				Value:      val.Value,
			}
			results.Values = append(results.Values, v)
		}

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
		inputFormat := &m2mapp.ResolveDataByAreaInput{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolvePastArea: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNode もしくは VMNode へリクエスト転送
		m2mapi_results := resolvePastAreaFunction(inputFormat.AD, inputFormat.NodeType, inputFormat.Capability, inputFormat.Period)
		// m2mapp用に変換
		results := m2mapp.ResolveDataByAreaOutput{}
		results.Values = make(map[string][]m2mapp.Value)
		for vnode_id, m2mapi_val := range m2mapi_results.Values {
			for _, m2mapp_val := range m2mapi_val {
				v := m2mapp.Value{
					Capability: m2mapp_val.Capability,
					Time:       m2mapp_val.Time,
					Value:      m2mapp_val.Value,
				}
				results.Values[vnode_id] = append(results.Values[vnode_id], v)
			}
		}

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
		inputFormat := &m2mapp.ResolveDataByAreaInput{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveCurrentArea: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNode もしくは VMNode へリクエスト転送
		m2mapi_results := resolveCurrentAreaFunction(inputFormat.AD, inputFormat.NodeType, inputFormat.Capability)
		// m2mapp用に成型
		results := m2mapp.ResolveDataByAreaOutput{}
		results.Values = make(map[string][]m2mapp.Value)
		for vnode_id, m2mapi_val := range m2mapi_results.Values {
			for _, m2mapp_val := range m2mapi_val {
				v := m2mapp.Value{
					Capability: m2mapp_val.Capability,
					Time:       m2mapp_val.Time,
					Value:      m2mapp_val.Value,
				}
				results.Values[vnode_id] = append(results.Values[vnode_id], v)
			}
		}

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
		inputFormat := &m2mapp.ResolveDataByAreaInput{}
		if err := json.Unmarshal(body, inputFormat); err != nil {
			http.Error(w, "resolveConditionArea: Error missmatching packet format", http.StatusInternalServerError)
		}

		// VNode もしくは VMNode へリクエスト転送
		m2mapi_results := resolveConditionAreaFunction(inputFormat.AD, inputFormat.NodeType, inputFormat.Capability, inputFormat.Condition)
		// m2mapp用に成型
		results := m2mapp.ResolveDataByAreaOutput{}
		results.Values = make(map[string][]m2mapp.Value)
		for vnode_id, m2mapi_val := range m2mapi_results.Values {
			for _, m2mapp_val := range m2mapi_val {
				v := m2mapp.Value{
					Capability: m2mapp_val.Capability,
					Time:       m2mapp_val.Time,
					Value:      m2mapp_val.Value,
				}
				results.Values[vnode_id] = append(results.Values[vnode_id], v)
			}
		}

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

func resolveAreaFunction(sw, ne m2mapp.SquarePoint) m2mapi.ResolveArea {
	// Cloud Sever に聞きに行くかのフラグと，エリア解決時に用いるServerIPのスライスの定義
	ask_cloud_flag := false
	var target_mec_server []string

	// 1. area_mapping_cache の情報と入力の矩形範囲を比べて，矩形範囲が area_mapping_cache の範囲と被っているか確認．被っている area_mapping がなければ，Cloud Serverに聞きに行く
	if len(area_mapping_cache) == 0 {
		ask_cloud_flag = true
	}
	for _, cover_area := range area_mapping_cache {
		if (ne.Lat <= cover_area.MinLat || ne.Lon <= cover_area.MinLon) || (sw.Lat >= cover_area.MaxLat || sw.Lon >= cover_area.MaxLon) {
			// 対象領域でない
			// 1つでもキャッシュされてない情報がある場合，Cloud Serverへ聞きに行く
			ask_cloud_flag = true
		} else {
			// 対象領域である
			target_mec_server = append(target_mec_server, cover_area.ServerIP)
		}
	}

	// 2. area_mapping_cache に情報がない，もしくは area_mapping_cache に対象の情報がない場合，Cloud Serverに対象サーバを聞きに行く
	if ask_cloud_flag {
		fmt.Println("Ask other MEC Server")
		area_mapping_data_request := m2mapi.AreaMapping{
			SW: m2mapi.SquarePoint{Lat: sw.Lat, Lon: sw.Lon},
			NE: m2mapi.SquarePoint{Lat: ne.Lat, Lon: ne.Lon},
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
			target_mec_server = addIfNotExists(target_mec_server, area_mapping.MECCoverArea.ServerIP)
		}
	}

	fmt.Println("target mec server: ", target_mec_server)

	// 3. 対象となったMEC ServerのLocal GraphDBに検索をかける．
	// この時，自MEC Serverに問い合わせる場合は，そのまま検索クエリを投げればいいが，他MEC Serverの場合，一度 M2M API を挟まなければいけない
	// 出力結果は，PAreaID, VNodeID, VNodeSocketAddress, VMNodeRSocketAddress
	payload := `{"statements": [{"statement": "MATCH (a:PArea)-[:contains]->(vs:VSNode) WHERE a.NE[0] > ` + strconv.FormatFloat(sw.Lat, 'f', 4, 64) + ` and a.NE[1] > ` + strconv.FormatFloat(sw.Lon, 'f', 4, 64) + ` and a.SW[0] < ` + strconv.FormatFloat(ne.Lat, 'f', 4, 64) + ` and a.SW[1] < ` + strconv.FormatFloat(ne.Lon, 'f', 4, 64) + ` return a.PAreaID, vs.VNodeID, vs.SocketAddress;"}]}`
	results := m2mapi.ResolveArea{}      // 最終的に M2M App に返す結果
	area_desc := m2mapi.AreaDescriptor{} // すべての結果を1つのADにまとめる
	area_desc.AreaDescriptorDetail = make(map[string]m2mapi.AreaDescriptorDetail)
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

			var row_data interface{}
			var area_desc_detail m2mapi.AreaDescriptorDetail
			for _, v1 := range values {
				for k2, v2 := range v1.(map[string]interface{}) {
					if k2 == "data" {
						for _, v3 := range v2.([]interface{}) {
							for k4, v4 := range v3.(map[string]interface{}) {
								if k4 == "row" {
									row_data = v4
									dataArray := row_data.([]interface{})
									area_desc_detail.PAreaID = addIfNotExists(area_desc_detail.PAreaID, dataArray[0].(string))
									vnode_set := m2mapi.VNodeSet{VNodeID: dataArray[1].(string), VNodeSocketAddress: dataArray[2].(string)}
									area_desc_detail.VNode = append(area_desc_detail.VNode, vnode_set)
									currentTime := time.Now()
									area_desc_detail.TTL = currentTime.Add(1 * time.Hour)
								}
							}
						}
					}
				}
			}
			area_desc.AreaDescriptorDetail[server_ip] = area_desc_detail
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
			transmit_response := m2mapi.ResolveArea{}
			if err = json.Unmarshal(body, &transmit_response); err != nil {
				fmt.Println("Error unmarshaling: ", err)
			}
			area_desc.AreaDescriptorDetail[server_ip] = transmit_response.Descriptor
		}
	}
	results.AreaDescriptor = area_desc

	return results
}

func resolveAreaTransmitFunction(sw, ne m2mapi.SquarePoint) m2mapi.ResolveArea {
	// M2M API からのリクエスト転送用の関数
	// 自MEC ServerのLocal GraphDBへの検索
	payload := `{"statements": [{"statement": "MATCH (a:PArea)-[:contains]->(vs:VSNode) WHERE a.NE[0] > ` + strconv.FormatFloat(sw.Lat, 'f', 4, 64) + ` and a.NE[1] > ` + strconv.FormatFloat(sw.Lon, 'f', 4, 64) + ` and a.SW[0] < ` + strconv.FormatFloat(ne.Lat, 'f', 4, 64) + ` and a.SW[1] < ` + strconv.FormatFloat(ne.Lon, 'f', 4, 64) + ` return a.PAreaID, vs.VNodeID, vs.SocketAddress;"}]}`
	graphdb_url := "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + ip_address + ":" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
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

	var row_data interface{}
	var area_desc_detail m2mapi.ResolveArea
	for _, v1 := range values {
		for k2, v2 := range v1.(map[string]interface{}) {
			if k2 == "data" {
				for _, v3 := range v2.([]interface{}) {
					for k4, v4 := range v3.(map[string]interface{}) {
						if k4 == "row" {
							row_data = v4
							dataArray := row_data.([]interface{})
							area_desc_detail.Descriptor.PAreaID = addIfNotExists(area_desc_detail.Descriptor.PAreaID, dataArray[0].(string))
							vnode_set := m2mapi.VNodeSet{VNodeID: dataArray[1].(string), VNodeSocketAddress: dataArray[2].(string)}
							area_desc_detail.Descriptor.VNode = append(area_desc_detail.Descriptor.VNode, vnode_set)
							currentTime := time.Now()
							area_desc_detail.TTL = currentTime.Add(1 * time.Hour)
						}
					}
				}
			}
		}
	}
	return area_desc_detail
}

func resolveNodeFunction(ad string, cap []string, node_type string) m2mapi.ResolveNode {
	// M2M App からの直接のリクエスト
	results := m2mapi.ResolveNode{}

	area_desc := ad_cache[ad]

	var format_capability []string
	for _, capability := range cap {
		capability = "\\\"" + capability + "\\\""
		format_capability = append(format_capability, capability)
	}

	if node_type == "All" || node_type == "VSNode" {
		for server_ip, ad_detail := range area_desc.AreaDescriptorDetail {
			if server_ip == ip_address {
				// 自MEC ServerのGraphDBでの検索
				var format_vsnodes []string
				for _, vsnode := range ad_detail.VNode {
					vnode_id := "\\\"" + vsnode.VNodeID + "\\\""
					format_vsnodes = append(format_vsnodes, vnode_id)
				}
				vnode_payload := `{"statements": [{"statement": "MATCH (vs:VSNode)-[:isPhysicalizedBy]->(ps:PSNode) WHERE vs.VNodeID IN [` + strings.Join(format_vsnodes, ", ") + `] and ps.Capability IN [` + strings.Join(format_capability, ", ") + `] return vs.VNodeID, vs.SocketAddress;"}]}`
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
				var vsnode_set_own m2mapi.VNodeSet
				for _, v1 := range values {
					for k2, v2 := range v1.(map[string]interface{}) {
						if k2 == "data" {
							for _, v3 := range v2.([]interface{}) {
								for k4, v4 := range v3.(map[string]interface{}) {
									if k4 == "row" {
										row_data = v4
										dataArray := row_data.([]interface{})
										vsnode_set_own.VNodeID = dataArray[0].(string)
										vsnode_set_own.VNodeSocketAddress = dataArray[1].(string)
										results.VNode = append(results.VNode, vsnode_set_own)
									}
								}
							}
						}
					}
				}

			} else {
				// 他MEC ServerのGraphDBでの検索
				transmit_m2mapi_url := "http://" + server_ip + ":" + os.Getenv("M2M_API_PORT") + "/m2mapi/node"
				var vnode_ids []string
				for _, vnode_id := range area_desc.AreaDescriptorDetail[server_ip].VNode {
					vnode_ids = append(vnode_ids, vnode_id.VNodeID)
				}
				// server_ip のAD情報だけ取り出す
				ad_detail_single := m2mapi.AreaDescriptor{}
				ad_detail_single.AreaDescriptorDetail = make(map[string]m2mapi.AreaDescriptorDetail)
				ad_detail_single.AreaDescriptorDetail[server_ip] = area_desc.AreaDescriptorDetail[server_ip]
				fmt.Println("20230928: area descriptor detail: ", ad_detail_single.AreaDescriptorDetail[server_ip])
				transmit_m2mapi_data := m2mapi.ResolveNode{
					AreaDescriptorDetail: ad_detail_single.AreaDescriptorDetail,
					Capability:           cap,
					NodeType:             node_type,
					TransmitFlag:         true,
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
					fmt.Println("Error reading response: ", err)
					panic(err)
				}
				transmit_response := m2mapi.ResolveNode{}
				if err = json.Unmarshal(body, &transmit_response); err != nil {
					fmt.Println("Error unmarshaling: ", err)
				}
				results.VNode = append(results.VNode, transmit_response.VNode...)
			}
		}
	}

	if node_type == "All" || node_type == "VMNode" {
		for server_ip, ad_detail := range area_desc.AreaDescriptorDetail {
			if server_ip == ip_address {
				// 自MEC ServerのGraphでの検索
				var format_areas []string
				for _, area := range ad_detail.PAreaID {
					area = "\\\"" + area + "\\\""
					format_areas = append(format_areas, area)
				}
				//vmnode_payload := `{"statements": [{"statement": "MATCH (a:PArea)-[:contains]->(vm:VMNode)-[:isPhysicalizedBy]->(pm:PMNode) WHERE a.PAreaID IN [` + strings.Join(format_areas, ", ") + `] and pm.Capability IN [` + strings.Join(format_capability, ", ") + `] return vm.VNodeID, vm.SocketAddress, vm.VMNodeRSocketAddress;"}]}`
				vmnode_payload := `{"statements": [{"statement": "MATCH (a:PArea)-[:contains]->(vm:VMNode) WHERE a.PAreaID IN [` + strings.Join(format_areas, ", ") + `] return vm.VNodeID, vm.SocketAddress, vm.VMNodeRSocketAddress;"}]}`
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
				var vmnode_set_own m2mapi.VNodeSet
				for _, v1 := range values {
					for k2, v2 := range v1.(map[string]interface{}) {
						if k2 == "data" {
							for _, v3 := range v2.([]interface{}) {
								for k4, v4 := range v3.(map[string]interface{}) {
									if k4 == "row" {
										row_data = v4
										dataArray := row_data.([]interface{})
										vmnode_set_own.VNodeID = dataArray[0].(string)
										vmnode_set_own.VNodeSocketAddress = dataArray[1].(string)
										vmnode_set_own.VMNodeRSocketAddress = dataArray[2].(string)
										results.VNode = append(results.VNode, vmnode_set_own)
									}
								}
							}
						}
					}
				}
			} else {
				// 他MEC ServerのGraphDBでの検索
				transmit_m2mapi_url := "http://" + server_ip + ":" + os.Getenv("M2M_API_PORT") + "/m2mapi/node"
				// server_ip のAD情報だけ取り出す
				ad_detail_single := m2mapi.AreaDescriptor{}
				ad_detail_single.AreaDescriptorDetail = make(map[string]m2mapi.AreaDescriptorDetail)
				ad_detail_single.AreaDescriptorDetail[server_ip] = area_desc.AreaDescriptorDetail[server_ip]
				transmit_m2mapi_data := m2mapi.ResolveNode{
					AreaDescriptorDetail: ad_detail_single.AreaDescriptorDetail,
					Capability:           cap,
					NodeType:             node_type,
					TransmitFlag:         true,
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
					fmt.Println("Error reading response: ", err)
					panic(err)
				}
				transmit_response := m2mapi.ResolveNode{}
				if err = json.Unmarshal(body, &transmit_response); err != nil {
					fmt.Println("Error unmarshaling: ", err)
				}
				results.VNode = append(results.VNode, transmit_response.VNode...)
			}
		}
	}
	return results
}

func resolveNodeTransmitFunction(ip_ad_detail map[string]m2mapi.AreaDescriptorDetail, capability []string, node_type string) m2mapi.ResolveNode {
	// M2M API からのリクエスト転送用の関数
	// 自MEC ServerのLocal GraphDBへの検索
	results := m2mapi.ResolveNode{}

	ad_detail := ip_ad_detail[ip_address]
	fmt.Println("20230928: ad_detail: ", ad_detail)

	var format_capability []string
	for _, cap := range capability {
		cap = "\\\"" + cap + "\\\""
		format_capability = append(format_capability, cap)
	}

	if node_type == "All" || node_type == "VSNode" {
		var format_vsnodes []string
		for _, vsnode_set := range ad_detail.VNode {
			vsnode := "\\\"" + vsnode_set.VNodeID + "\\\""
			format_vsnodes = append(format_vsnodes, vsnode)
		}

		vnode_payload := `{"statements": [{"statement": "MATCH (vs:VSNode)-[:isPhysicalizedBy]->(ps:PSNode) WHERE vs.VNodeID IN [` + strings.Join(format_vsnodes, ", ") + `] and ps.Capability IN [` + strings.Join(format_capability, ", ") + `] return vs.VNodeID, vs.SocketAddress;"}]}`
		graphdb_url := "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + ip_address + ":" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
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
		var vsnode_set_own m2mapi.VNodeSet
		for _, v1 := range values {
			for k2, v2 := range v1.(map[string]interface{}) {
				if k2 == "data" {
					for _, v3 := range v2.([]interface{}) {
						for k4, v4 := range v3.(map[string]interface{}) {
							if k4 == "row" {
								row_data = v4
								dataArray := row_data.([]interface{})
								vsnode_set_own.VNodeID = dataArray[0].(string)
								vsnode_set_own.VNodeSocketAddress = dataArray[1].(string)
								results.VNode = append(results.VNode, vsnode_set_own)
							}
						}
					}
				}
			}
		}
	}

	if node_type == "All" || node_type == "VMNode" {
		var format_areas []string
		for _, area := range ad_detail.PAreaID {
			area = "\\\"" + area + "\\\""
			format_areas = append(format_areas, area)
		}
		vmnode_payload := `{"statements": [{"statement": "MATCH (a:PArea)-[:contains]->(vm:VMNode)-[:isPhysicalizedBy]->(pm:PMNode) WHERE a.PAreaID IN [` + strings.Join(format_areas, ", ") + `] and pm.Capability IN [` + strings.Join(format_capability, ", ") + `] return vm.VNodeID, vm.SocketAddress, vm.VMNodeRSocketAddress;"}]}`
		graphdb_url := "http://" + os.Getenv("NEO4J_USERNAME") + ":" + os.Getenv("NEO4J_LOCAL_PASSWORD") + "@" + ip_address + ":" + os.Getenv("NEO4J_LOCAL_PORT_GOLANG") + "/db/data/transaction/commit"
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
		var vmnode_set_own m2mapi.VNodeSet
		for _, v1 := range values {
			for k2, v2 := range v1.(map[string]interface{}) {
				if k2 == "data" {
					for _, v3 := range v2.([]interface{}) {
						for k4, v4 := range v3.(map[string]interface{}) {
							if k4 == "row" {
								row_data = v4
								dataArray := row_data.([]interface{})
								vmnode_set_own.VNodeID = dataArray[0].(string)
								vmnode_set_own.VNodeSocketAddress = dataArray[1].(string)
								vmnode_set_own.VMNodeRSocketAddress = dataArray[2].(string)
								results.VNode = append(results.VNode, vmnode_set_own)
							}
						}
					}
				}
			}
		}
	}

	return results
}

func resolveNodePMNodeFunction(ip_ad_detail map[string]m2mapi.AreaDescriptorDetail, cap []string, node_type string) m2mapi.ResolveNode {
	//PMNode M2M API からのリクエスト転送
	results := m2mapi.ResolveNode{}

	var format_capability []string
	for _, capability := range cap {
		capability = "\\\"" + capability + "\\\""
		format_capability = append(format_capability, capability)
	}

	if node_type == "All" || node_type == "VSNode" {
		for server_ip, ad_detail := range ip_ad_detail {
			if server_ip == ip_address {
				// 自MEC ServerのGraphDBでの検索
				var format_vsnodes []string
				for _, vsnode := range ad_detail.VNode {
					vnode_id := "\\\"" + vsnode.VNodeID + "\\\""
					format_vsnodes = append(format_vsnodes, vnode_id)
				}
				vnode_payload := `{"statements": [{"statement": "MATCH (vs:VSNode)-[:isPhysicalizedBy]->(ps:PSNode) WHERE vs.VNodeID IN [` + strings.Join(format_vsnodes, ", ") + `] and ps.Capability IN [` + strings.Join(format_capability, ", ") + `] return vs.VNodeID, vs.SocketAddress;"}]}`
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
				var vsnode_set_own m2mapi.VNodeSet
				for _, v1 := range values {
					for k2, v2 := range v1.(map[string]interface{}) {
						if k2 == "data" {
							for _, v3 := range v2.([]interface{}) {
								for k4, v4 := range v3.(map[string]interface{}) {
									if k4 == "row" {
										row_data = v4
										dataArray := row_data.([]interface{})
										vsnode_set_own.VNodeID = dataArray[0].(string)
										vsnode_set_own.VNodeSocketAddress = dataArray[1].(string)
										results.VNode = append(results.VNode, vsnode_set_own)
									}
								}
							}
						}
					}
				}

			} else {
				// 他MEC ServerのGraphDBでの検索
				transmit_m2mapi_url := "http://" + server_ip + ":" + os.Getenv("M2M_API_PORT") + "/m2mapi/node"
				// server_ip のAD情報だけ取り出す
				ad_detail_single := m2mapi.AreaDescriptor{}
				ad_detail_single.AreaDescriptorDetail = make(map[string]m2mapi.AreaDescriptorDetail)
				ad_detail_single.AreaDescriptorDetail[server_ip] = ad_detail
				transmit_m2mapi_data := m2mapi.ResolveNode{
					AreaDescriptorDetail: ad_detail_single.AreaDescriptorDetail,
					Capability:           cap,
					NodeType:             node_type,
					TransmitFlag:         true,
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
					fmt.Println("Error reading response: ", err)
					panic(err)
				}
				transmit_response := m2mapi.ResolveNode{}
				if err = json.Unmarshal(body, &transmit_response); err != nil {
					fmt.Println("Error unmarshaling: ", err)
				}
				results.VNode = append(results.VNode, transmit_response.VNode...)
			}
		}
	}

	if node_type == "All" || node_type == "VMNode" {
		for server_ip, ad_detail := range ip_ad_detail {
			if server_ip == ip_address {
				// 自MEC ServerのGraphでの検索
				var format_areas []string
				for _, area := range ad_detail.PAreaID {
					area = "\\\"" + area + "\\\""
					format_areas = append(format_areas, area)
				}
				vmnode_payload := `{"statements": [{"statement": "MATCH (a:PArea)-[:contains]->(vm:VMNode)-[:isPhysicalizedBy]->(pm:PMNode) WHERE a.PAreaID IN [` + strings.Join(format_areas, ", ") + `] and pm.Capability IN [` + strings.Join(format_capability, ", ") + `] return vm.VNodeID, vm.SocketAddress, vm.VMNodeRSocketAddress;"}]}`
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
				var vmnode_set_own m2mapi.VNodeSet
				for _, v1 := range values {
					for k2, v2 := range v1.(map[string]interface{}) {
						if k2 == "data" {
							for _, v3 := range v2.([]interface{}) {
								for k4, v4 := range v3.(map[string]interface{}) {
									if k4 == "row" {
										row_data = v4
										dataArray := row_data.([]interface{})
										vmnode_set_own.VNodeID = dataArray[0].(string)
										vmnode_set_own.VNodeSocketAddress = dataArray[1].(string)
										vmnode_set_own.VMNodeRSocketAddress = dataArray[2].(string)
										results.VNode = append(results.VNode, vmnode_set_own)
									}
								}
							}
						}
					}
				}
			} else {
				// 他MEC ServerのGraphDBでの検索
				transmit_m2mapi_url := "http://" + server_ip + ":" + os.Getenv("M2M_API_PORT") + "/m2mapi/node"
				// server_ip のAD情報だけ取り出す
				ad_detail_single := m2mapi.AreaDescriptor{}
				ad_detail_single.AreaDescriptorDetail = make(map[string]m2mapi.AreaDescriptorDetail)
				ad_detail_single.AreaDescriptorDetail[server_ip] = ad_detail
				transmit_m2mapi_data := m2mapi.ResolveNode{
					AreaDescriptorDetail: ad_detail_single.AreaDescriptorDetail,
					Capability:           cap,
					NodeType:             node_type,
					TransmitFlag:         true,
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
					fmt.Println("Error reading response: ", err)
					panic(err)
				}
				transmit_response := m2mapi.ResolveNode{}
				if err = json.Unmarshal(body, &transmit_response); err != nil {
					fmt.Println("Error unmarshaling: ", err)
				}
				results.VNode = append(results.VNode, transmit_response.VNode...)
			}
		}
	}
	return results
}

func resolvePastNodeFunction(vnode_id, socket_address string, capability []string, period m2mapp.PeriodInput) m2mapi.ResolveDataByNode {
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

func resolveCurrentNodeFunction(vnode_id, socket_address string, capability []string) m2mapi.ResolveDataByNode {
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
	fmt.Println("receive from vsnode: ", results)

	return results
}

func resolveConditionNodeFunction(vnode_id, socket_address string, capability []string, condition m2mapp.ConditionInput) m2mapi.ResolveDataByNode {
	null_data := m2mapi.ResolveDataByNode{VNodeID: "NULL"}

	request_data := m2mapi.ResolveDataByNode{
		VNodeID:    vnode_id,
		Capability: capability,
		Condition:  m2mapi.ConditionInput{Limit: m2mapi.Range{LowerLimit: condition.Limit.LowerLimit, UpperLimit: condition.Limit.UpperLimit}, Timeout: condition.Timeout},
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

func resolvePastAreaFunction(ad, node_type string, capability []string, period m2mapp.PeriodInput) m2mapi.ResolveDataByArea {
	var results m2mapi.ResolveDataByArea
	results.Values = make(map[string][]m2mapi.Value)

	// データ取得対象となるVNode群の検索
	resolve_node_results := resolveNodeFunction(ad, capability, node_type)

	// resolveNodeの検索によって得られたすべてのVNodeに対してデータ取得要求
	if node_type == "All" || node_type == "VSNode" {
		var wg sync.WaitGroup
		for _, vsnode_set := range resolve_node_results.VNode {
			wg.Add(1)
			go func(vsnode_set m2mapi.VNodeSet) {
				defer wg.Done()
				request_data := m2mapi.ResolveDataByNode{
					VNodeID:    vsnode_set.VNodeID,
					Capability: capability,
					Period:     m2mapi.PeriodInput{Start: period.Start, End: period.End},
				}

				transmit_data, err := json.Marshal(request_data)
				if err != nil {
					fmt.Println("Error marshaling data: ", err)
					results.AD = "NULL"
				}
				transmit_url := "http://" + vsnode_set.VNodeSocketAddress + "/primapi/data/past/node"
				response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
				if err != nil {
					fmt.Println("Error making request: ", err)
					results.AD = "NULL"
				}
				defer response_data.Body.Close()

				byteArray, _ := io.ReadAll(response_data.Body)
				var result m2mapi.ResolveDataByNode
				if err := json.Unmarshal(byteArray, &result); err != nil {
					fmt.Println("Error unmarshaling data: ", err)
					results.AD = "NULL"
				}

				results.Values[vsnode_set.VNodeID] = append(results.Values[vsnode_set.VNodeID], result.Values...)
			}(vsnode_set)
		}
		wg.Wait()
	}

	if node_type == "All" || node_type == "VMNode" {
		var wg sync.WaitGroup
		for _, vmnode_set := range resolve_node_results.VNode {
			wg.Add(1)
			go func(vmnode_set m2mapi.VNodeSet) {
				defer wg.Done()
				request_data := m2mapi.ResolveDataByNode{
					VNodeID:    vmnode_set.VNodeID,
					Capability: capability,
					Period:     m2mapi.PeriodInput{Start: period.Start, End: period.End},
				}

				transmit_data, err := json.Marshal(request_data)
				if err != nil {
					fmt.Println("Error marhsaling data: ", err)
					results.AD = "NULL"
				}
				transmit_url := "http://" + vmnode_set.VNodeSocketAddress + "/primapi/data/past/node"
				response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
				if err != nil {
					fmt.Println("Error making request: ", err)
					results.AD = "NULL"
				}
				defer response_data.Body.Close()

				byteArray, _ := io.ReadAll(response_data.Body)
				var result m2mapi.ResolveDataByNode
				if err := json.Unmarshal(byteArray, &result); err != nil {
					fmt.Println("Error unmarshaling data: ", err)
					results.AD = "NULL"
				}

				results.Values[vmnode_set.VNodeID] = append(results.Values[vmnode_set.VNodeID], result.Values...)
			}(vmnode_set)
		}
		wg.Wait()
	}

	return results
}

func resolveCurrentAreaFunction(ad, node_type string, capability []string) m2mapi.ResolveDataByArea {
	var results m2mapi.ResolveDataByArea
	results.Values = make(map[string][]m2mapi.Value)

	// データ取得対象となるVNode群の検索
	resolve_node_results := resolveNodeFunction(ad, capability, node_type)
	fmt.Println("resolve_node_results: ", resolve_node_results)

	// resolveNodeの検索によって得られたすべてのVNodeに対してデータ取得要求
	if node_type == "All" || node_type == "VSNode" {
		var wg sync.WaitGroup
		for _, vsnode_set := range resolve_node_results.VNode {
			wg.Add(1)
			go func(vsnode_set m2mapi.VNodeSet) {
				defer wg.Done()
				request_data := m2mapi.ResolveDataByNode{
					VNodeID:    vsnode_set.VNodeID,
					Capability: capability,
				}

				transmit_data, err := json.Marshal(request_data)
				if err != nil {
					fmt.Println("Error marshaling data: ", err)
					results.AD = "NULL"
				}
				// VSNodeへ転送
				transmit_url := "http://" + vsnode_set.VNodeSocketAddress + "/primapi/data/current/node"
				response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
				if err != nil {
					fmt.Println("Error making request: ", err)
					results.AD = "NULL"
				}
				defer response_data.Body.Close()

				byteArray, _ := io.ReadAll(response_data.Body)
				var result m2mapi.ResolveDataByNode
				if err := json.Unmarshal(byteArray, &result); err != nil {
					fmt.Println("Error unmarshaling data: ", err)
					results.AD = "NULL"
				}

				results.Values[vsnode_set.VNodeID] = append(results.Values[vsnode_set.VNodeID], result.Values...)
			}(vsnode_set)
		}
		wg.Wait()
	}

	if node_type == "All" || node_type == "VMNode" {
		var wg sync.WaitGroup
		for _, vmnode_set := range resolve_node_results.VNode {
			wg.Add(1)
			go func(vmnode_set m2mapi.VNodeSet) {
				defer wg.Done()
				request_data := m2mapi.ResolveDataByNode{
					VNodeID:    vmnode_set.VNodeID,
					Capability: capability,
				}

				transmit_data, err := json.Marshal(request_data)
				if err != nil {
					fmt.Println("Error marhsaling data: ", err)
					results.AD = "NULL"
				}
				// VSNodeへ転送
				transmit_url := "http://" + vmnode_set.VMNodeRSocketAddress + "/primapi/data/current/node"
				response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
				if err != nil {
					fmt.Println("Error making request: ", err)
					results.AD = "NULL"
				}
				defer response_data.Body.Close()

				byteArray, _ := io.ReadAll(response_data.Body)
				var result m2mapi.ResolveDataByNode
				if err := json.Unmarshal(byteArray, &result); err != nil {
					fmt.Println("Error unmarshaling data: ", err)
					results.AD = "NULL"
				}

				results.Values[vmnode_set.VNodeID] = append(results.Values[vmnode_set.VNodeID], result.Values...)
			}(vmnode_set)
		}
		wg.Wait()
	}

	return results
}

func resolveConditionAreaFunction(ad, node_type string, capability []string, condition m2mapp.ConditionInput) m2mapi.ResolveDataByArea {
	var results m2mapi.ResolveDataByArea
	results.Values = make(map[string][]m2mapi.Value)

	// データ取得対象となるVNode群の検索
	resolve_node_results := resolveNodeFunction(ad, capability, node_type)

	// resolveNodeの検索によって得られたすべてのVNodeに対してデータ取得要求
	if node_type == "All" || node_type == "VSNode" {
		var wg sync.WaitGroup
		for _, vsnode_set := range resolve_node_results.VNode {
			wg.Add(1)
			go func(vsnode_set m2mapi.VNodeSet) {
				defer wg.Done()

				request_data := m2mapi.ResolveDataByNode{
					VNodeID:    vsnode_set.VNodeID,
					Capability: capability,
					Condition:  m2mapi.ConditionInput{Limit: m2mapi.Range{LowerLimit: condition.Limit.LowerLimit, UpperLimit: condition.Limit.UpperLimit}, Timeout: condition.Timeout},
				}

				transmit_data, err := json.Marshal(request_data)
				if err != nil {
					fmt.Println("Error marshaling data: ", err)
					results.AD = "NULL"
				}
				// VSNodeへ転送
				transmit_url := "http://" + vsnode_set.VNodeSocketAddress + "/primapi/data/condition/node"
				response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
				if err != nil {
					fmt.Println("Error making request: ", err)
					results.AD = "NULL"
				}
				defer response_data.Body.Close()

				byteArray, _ := io.ReadAll(response_data.Body)
				var result m2mapi.ResolveDataByNode
				if err := json.Unmarshal(byteArray, &result); err != nil {
					fmt.Println("Error unmarshaling data: ", err)
					results.AD = "NULL"
				}

				results.Values[vsnode_set.VNodeID] = append(results.Values[vsnode_set.VNodeID], result.Values...)
				fmt.Println("other process cancel")

			}(vsnode_set)
		}
		wg.Wait()
	}

	if node_type == "All" || node_type == "VMNode" {
		var wg sync.WaitGroup
		for _, vmnode_set := range resolve_node_results.VNode {
			wg.Add(1)
			go func(vmnode_set m2mapi.VNodeSet) {
				defer wg.Done()
				request_data := m2mapi.ResolveDataByNode{
					VNodeID:    vmnode_set.VNodeID,
					Capability: capability,
					Condition:  m2mapi.ConditionInput{Limit: m2mapi.Range{LowerLimit: condition.Limit.LowerLimit, UpperLimit: condition.Limit.UpperLimit}, Timeout: condition.Timeout},
				}

				transmit_data, err := json.Marshal(request_data)
				if err != nil {
					fmt.Println("Error marhsaling data: ", err)
					results.AD = "NULL"
				}
				// VMNodeRへ転送
				transmit_url := "http://" + vmnode_set.VMNodeRSocketAddress + "/primapi/data/condition/node"
				response_data, err := http.Post(transmit_url, "application/json", bytes.NewBuffer(transmit_data))
				if err != nil {
					fmt.Println("Error making request: ", err)
					results.AD = "NULL"
				}
				defer response_data.Body.Close()

				byteArray, _ := io.ReadAll(response_data.Body)
				var result m2mapi.ResolveDataByNode
				if err := json.Unmarshal(byteArray, &result); err != nil {
					fmt.Println("Error unmarshaling data: ", err)
					results.AD = "NULL"
				}

				results.Values[vmnode_set.VNodeID] = append(results.Values[vmnode_set.VNodeID], result.Values...)
			}(vmnode_set)
		}
		wg.Wait()
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
