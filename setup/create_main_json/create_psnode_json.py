import json
from dotenv import load_dotenv
import os
import math
import random

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/Main/config/json_files"

#VSNODE_BASE_PORT
#VPOINT_NUM_PER_AREA
#PSINK_NUM_PER_VPOINT
#PSNODE_NUM_PER_PSINK
#MIN_LAT, MAX_LAT, MIN_LON, MAX_LON
#AREA_WIDTH
#EDGE_SERVER_NUM
VSNODE_BASE_PORT = 3000
VPOINT_NUM_PER_AREA = 1
PSINK_NUM_PER_VPOINT = 1
#PSNODE_NUM_PER_PSINK = 2
VSNODE_NUM_PER_PNTYPE = 1
MIN_LAT = 35.530
MAX_LAT = 35.540
MIN_LON = 139.530
MAX_LON = 139.540
AREA_WIDTH = 0.001
EDGE_SERVER_NUM = 3

lineStep = AREA_WIDTH
forint = 1000

area_num = math.ceil(((MAX_LAT-MIN_LAT)/AREA_WIDTH)*((MAX_LON-MIN_LON)/AREA_WIDTH))
area_num_per_server = int(area_num / EDGE_SERVER_NUM)

data = {"psnodes":{"psnode":[], "vsnode":[]}}

#始点となるArea
swLat = MIN_LAT
neLat = swLat + lineStep

#label情報
label_lat = 0
label_lon = 0

#server_counter
server_counter = 0
server_num = 1

#ServerごとのPSinkの番号
psink_num = 0

#PSinkごとのPSNodeの番号
psnode_num = 0
port_num = 0

#PSNodeのIPv6Pref用のインデックス
psnode_ipv6_pref = 0

#PNTypeをあらかじめ用意
pn_types = ["Temp_Sensor", "Humid_Sensor", "Anemometer"]
capabilities = {"Temp_Sensor":"MaxTemp", "Humid_Sensor":"MaxHumid", "Anemometer":"MaxWind"}
vsnode_psnode_relation = {}
for i in range(len(pn_types)):
    vsnode_psnode_relation[pn_types[i]] = []

#PSNodeの設定
#左下からスタートし，右へ進んでいく
#端まで到達したら一段上へ
while neLat <= MAX_LAT:
    swLon = MIN_LON
    neLon = swLon + lineStep
    label_lon = 0
    while neLon <= MAX_LON:
        server_counter += 1
        if server_num > EDGE_SERVER_NUM:
            server_num -= 1
        if server_counter == area_num_per_server*server_num+1 and server_num < EDGE_SERVER_NUM:
            psink_num = 0
            server_num += 1
        if (area_num_per_server*(server_num-1)) <= server_counter < (area_num_per_server*server_num):
            label_server = "S" + str(server_num)
        i = 0
        while i < VPOINT_NUM_PER_AREA:
            psink_covered_area = []
            j = 0
            for pntype in range(len(pn_types)):
                vsnode_psnode_relation[pn_types[pntype]] = []
            while j < PSINK_NUM_PER_VPOINT:
                label_psink = "PS" + str(server_num) + ":" + str(psink_num)
                psnode_num = 0
                k = 0
                while k < len(pn_types):
                    #PSNode情報の追加
                    label_psnode = "PSN" + str(server_num) + ":" + str(psink_num) + ":" + str(psnode_num)
                    #psnode_id = "PSN" + str(server_num) + "_" + str(psink_num) + "_" + str(psnode_num)
                    psnode_lat = random.uniform(swLat, neLat)
                    psnode_lon = random.uniform(swLon, neLon)
                    psnode_socket_file = "/tmp/mecm2m/psnode_" + str(server_num) + "_" + str(psink_num) + "_" + str(psnode_num) + ".sock"
                    psnode_pn_type = pn_types[k]
                    psnode_dict = {
                        "property-label": "PSNode",
                        "relation-label": {
                            "PSink": label_psink,
                            "PNodeID": str(psnode_ipv6_pref),
                            "socket-file": psnode_socket_file
                        },
                        "data-property": {
                            "Label": label_psnode,
                            "PNodeID": str(psnode_ipv6_pref),
                            "IPv6Pref": str(psnode_ipv6_pref),
                            "PNType": psnode_pn_type,
                            "Position": [round(psnode_lat, 4), round(psnode_lon, 4)],
                            "Capability": capabilities[psnode_pn_type],
                            "Credential": "YES",
                            "Description": "PSNode" + label_psnode
                        },
                        "object-property": [
                            {
                                "from": {
                                    "property-label": "PSNode",
                                    "data-property": "Label",
                                    "value": label_psnode
                                },
                                "to": {
                                    "property-label": "PSink",
                                    "data-property": "Label",
                                    "value": label_psink
                                },
                                "type": "respondsViaDevApi"
                            },
                            {
                                "from": {
                                    "property-label": "PSink",
                                    "data-property": "Label",
                                    "value": label_psink
                                },
                                "to": {
                                    "property-label": "PSNode",
                                    "data-property": "Label",
                                    "value": label_psnode
                                },
                                "type": "requestsViaDevApi"
                            }
                        ]
                    }
                    data["psnodes"]["psnode"].append(psnode_dict)
                    vsnode_psnode_relation[psnode_pn_type].append(label_psnode)
                    psnode_num += 1
                    psnode_ipv6_pref += 1
                    k += 1
                psink_covered_area.append(psink_num)
                psink_num += 1
                j += 1
            #VPointの情報を生成
            if PSINK_NUM_PER_VPOINT == 1:
                label_vpoint = str(server_num) + ":" + str(psink_covered_area[0])
                vpoint_id = str(server_num) + "_" + str(psink_covered_area[0])
            else:
                label_vpoint = str(server_num) + ":" + str(psink_covered_area[0]) + "-" + str(psink_covered_area[-1])
                vpoint_id = str(server_num) + "_" + str(psink_covered_area[0]) + "-" + str(psink_covered_area[-1])
            #VSNode情報の追加
            l = 0
            vsnode_num = 0
            while l < VSNODE_NUM_PER_PNTYPE:
                m = 0
                while m < len(pn_types):
                    label_vsnode = "VSN" + label_vpoint + ":" + str(vsnode_num)
                    vsnode_id = "VSN" + vpoint_id + "_" + str(vsnode_num)
                    port = VSNODE_BASE_PORT + port_num
                    vsnode_dict = {
                        "property-label": "VSNode",
                        "data-property": {
                        "Label": label_vsnode,
                        "VNodeID": vsnode_id,
                        "VNType": pn_types[m],
                        "Port": str(port),
                        "Description": "VSNode" + label_vsnode
                        },
                        "object-property": [
                        
                        ]
                    }
                    data["psnodes"]["vsnode"].append(vsnode_dict)
                    for pntype in vsnode_psnode_relation[pn_types[m]]:
                        isComposedOf_object_property = {
                            "from": {
                                "property-label": "VSNode",
                                "data-property": "Label",
                                "value": label_vsnode
                            },
                            "to": {
                                "property-label": "PSNode",
                                "data-property": "Label",
                                "value": pntype
                            },
                            "type": "isComposedOf"
                        }
                        isVirtualizedWith_object_property = {
                            "from": {
                                "property-label": "PSNode",
                                "data-property": "Label",
                                "value": pntype
                            },
                            "to": {
                                "property-label": "VSNode",
                                "data-property": "Label",
                                "value": label_vsnode
                            },
                            "type": "isVirtualizedWith"
                        }
                        data["psnodes"]["vsnode"][-1]["object-property"].append(isComposedOf_object_property)
                        data["psnodes"]["vsnode"][-1]["object-property"].append(isVirtualizedWith_object_property)
                    vsnode_num += 1
                    port_num += 1
                    m += 1
                l += 1
            i += 1
        label_lon += 1
        swLon = ((swLon*forint) + (lineStep*forint)) / forint
        neLon = ((neLon*forint) + (lineStep*forint)) / forint
    label_lat += 1
    swLat = ((swLat*forint) + (lineStep*forint)) / forint
    neLat = ((neLat*forint) + (lineStep*forint)) / forint


psnode_json = json_file_path + "/config_main_psnode.json"
with open(psnode_json, 'w') as f:
    json.dump(data, f, indent=4)