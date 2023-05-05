import json
from dotenv import load_dotenv
import os
import math
import random

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/Main/config/json_files"

#VPOINT_BASE_PORT
#VPOINT_NUM_PER_AREA
#PSINK_NUM_PER_VPOINT
#MIN_LAT, MAX_LAT, MIN_LON, MAX_LON
#AREA_WIDTH
#EDGE_SERVER_NUM
VPOINT_BASE_PORT = 2000
VPOINT_NUM_PER_AREA = 1
PSINK_NUM_PER_VPOINT = 1
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

data = {"psinks":[]}

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

#VPointのPort番号
port_num = 0

#左下からスタートし，右へ進んでいく
#端まで到達したら一段上へ
while neLat <= MAX_LAT:
    swLon = MIN_LON
    neLon = swLon + lineStep
    label_lon = 0
    while neLon <= MAX_LON:
        #data["psinks"].append({"psink":[], "vpoint":[]})
        #x = data["psinks"][len(data["psinks"])-1]["psink"]
        #y = data["psinks"][len(data["psinks"])-1]["vpoint"]
        label_area = "A" + str(label_lat) + ":" + str(label_lon)
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
            data["psinks"].append({"psink":[], "vpoint":[]})
            x = data["psinks"][len(data["psinks"])-1]["psink"]
            y = data["psinks"][len(data["psinks"])-1]["vpoint"]
            psink_covered_area = []
            j = 0
            while j < PSINK_NUM_PER_VPOINT:
                #print(count)
                #PSink情報の追加
                label_psink = "PS" + str(server_num) + ":" + str(psink_num)
                psink_id = "PS" + str(server_num) + "_" + str(psink_num)
                psink_lat = random.uniform(swLat, neLat)
                psink_lon = random.uniform(swLon, neLon)
                psink_dict = {
                    "property-label": "PSink",
                    "relation-label": {
                        "Server": label_server,
                        "Area": label_area
                    },
                    "data-property": {
                        "Label": label_psink,
                        "ServingIPv6Address": "",#適当なサブネットマスクを生成する
                        "PSinkID": psink_id,
                        "Position": [round(psink_lat, 4), round(psink_lon, 4)],
                        "Description": "PSink" + label_psink
                    },
                    "object-property": [
                        {
                            "from": {
                                "property-label": "PSink",
                                "data-property": "Label",
                                "value": label_psink
                            },
                            "to": {
                                "property-label": "Area",
                                "data-property": "Label",
                                "value": label_area
                            },
                            "type": "isInstalledIn"
                        },
                        {
                            "from": {
                                "property-label": "Area",
                                "data-property": "Label",
                                "value": label_area
                            },
                            "to": {
                                "property-label": "PSink",
                                "data-property": "Label",
                                "value": label_psink
                            },
                            "type": "contains"
                        }
                    ]
                }
                x.append(psink_dict)
                psink_covered_area.append(psink_num)
                psink_num += 1
                j += 1
            #VPoint情報の追加
            if PSINK_NUM_PER_VPOINT == 1:
                label_vpoint = "VP" + str(server_num) + ":" + str(psink_covered_area[0])
                vpoint_id = "VP" + str(server_num) + "_" + str(psink_covered_area[0])
            else:
                label_vpoint = "VP" + str(server_num) + ":" + str(psink_covered_area[0]) + "-" + str(psink_covered_area[-1])
                vpoint_id = "VP" + str(server_num) + "_" + str(psink_covered_area[0]) + "-" + str(psink_covered_area[-1])
            port = VPOINT_BASE_PORT + port_num
            vpoint_dict = {
                "property-label": "VPoint",
                "data-property": {
                    "Label": label_vpoint,
                    "VPointID": vpoint_id,
                    "Port": str(port),
                    "Description": "VPoint" + label_vpoint
                },
                "object-property": [
                
                ]
            }
            y.append(vpoint_dict)
            for i in psink_covered_area:
                label_psink_for_vpoint = "PS" + str(server_num) + ":" + str(i)
                isComposedOf_object_property = {
                    "from": {
                        "property-label": "VPoint",
                        "data-property": "Label",
                        "value": label_vpoint
                    },
                    "to": {
                        "property-label": "PSink",
                        "data-property": "Label",
                        "value": label_psink_for_vpoint
                    },
                    "type": "isComposedOf"
                }
                isVirtualizedWith_object_property = {
                    "from": {
                        "property-label": "PSink",
                        "data-property": "Label",
                        "value": label_psink_for_vpoint
                    },
                    "to": {
                        "property-label": "VPoint",
                        "data-property": "Label",
                        "value": label_vpoint
                    },
                    "type": "isVirtualizedWith"
                }
                isRunningOn_object_property = {
                    "from": {
                        "property-label": "VPoint",
                        "data-property": "Label",
                        "value": label_vpoint
                    },
                    "to": {
                        "property-label": "Server",
                        "data-property": "Label",
                        "value": label_server
                    },
                    "type": "isRunningOn"
                }
                supports_object_property = {
                    "from": {
                        "property-label": "Server",
                        "data-property": "Label",
                        "value": label_server
                    },
                    "to": {
                        "property-label": "VPoint",
                        "data-property": "Label",
                        "value": label_vpoint
                    },
                    "type": "supports"
                }
                y[0]["object-property"].append(isComposedOf_object_property)
                y[0]["object-property"].append(isVirtualizedWith_object_property)
                y[0]["object-property"].append(isRunningOn_object_property)
                y[0]["object-property"].append(supports_object_property)
            port_num += 1
            i += 1
        label_lon += 1
        swLon = ((swLon*forint) + (lineStep*forint)) / forint
        neLon = ((neLon*forint) + (lineStep*forint)) / forint
    label_lat += 1
    swLat = ((swLat*forint) + (lineStep*forint)) / forint
    neLat = ((neLat*forint) + (lineStep*forint)) / forint

psink_json = json_file_path + "/config_main_psink.json"
with open(psink_json, 'w') as f:
    json.dump(data, f, indent=4)