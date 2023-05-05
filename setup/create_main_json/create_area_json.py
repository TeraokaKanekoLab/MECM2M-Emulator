import json
from dotenv import load_dotenv
import os
import random

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/Main/config/json_files"

#MIN_LAT, MAX_LAT, MIN_LON, MAX_LON
#AREA_WIDTH
MIN_LAT = 35.530
MAX_LAT = 35.540
MIN_LON = 139.530
MAX_LON = 139.540
AREA_WIDTH = 0.001

lineStep = AREA_WIDTH
forint = 1000

data = {"areas":{"area":[]}}

#始点となるArea
swLat = MIN_LAT
neLat = swLat + lineStep

#label情報
label_lat = 0
label_lon = 0

#左下からスタートし，右へ進んでいく
#端まで到達したら一段上へ
while neLat <= MAX_LAT:
    swLon = MIN_LON
    neLon = swLon + lineStep
    label_lon = 0
    while neLon <= MAX_LON:
        #area = {}
        area = data["areas"]["area"]
        label = "A" + str(label_lat) + ":" + str(label_lon)
        area_id = str(int(swLat*1000)) + "0" + str(int(swLon*1000)) + "0"

        area_dict = {
            "property-label": "Area",
            "data-property": {
                "Label": label,
                "Area": area_id,
                "SW": [round(swLat, 3), round(swLon, 3)],
                "NE": [round(neLat, 3), round(neLon, 3)],
                "Address": "Hiyoshi, Kohoku, Yokohama, Kanagawa",
                "Description": "Area" + area_id
            },
            "object-property": [
            
            ]
        }
        object_properties = area_dict["object-property"]
        if label_lat > 0:
            #isNorthOf, isSouthOf
            isSouth_label = "A" + str(label_lat-1) + ":" + str(label_lon)
            isNorthOf_object_property = {
                "from": {
                    "property-label": "Area",
                    "data-property": "Label",
                    "value": label
                },
                "to": {
                    "property-label": "Area",
                    "data-property": "Label",
                    "value": isSouth_label
                },
                "type": "isNorthOf"
            }
            isSouthOf_object_property = {
                "from": {
                    "property-label": "Area",
                    "data-property": "Label",
                    "value": isSouth_label
                },
                "to": {
                    "property-label": "Area",
                    "data-property": "Label",
                    "value": label
                },
                "type": "isSouthOf"
            }
            object_properties.append(isNorthOf_object_property)
            object_properties.append(isSouthOf_object_property)
        if label_lon > 0:
            #isEastOf, isWestOf
            isWest_label = "A" + str(label_lat) + ":" + str(label_lon-1)
            isEastOf_object_property = {
                "from": {
                    "property-label": "Area",
                    "data-property": "Label",
                    "value": label
                },
                "to": {
                    "property-label": "Area",
                    "data-property": "Label",
                    "value": isWest_label
                },
                "type": "isEastOf"
            }
            isWestOf_object_property = {
                "from": {
                    "property-label": "Area",
                    "data-property": "Label",
                    "value": isWest_label
                },
                "to": {
                    "property-label": "Area",
                    "data-property": "Label",
                    "value": label
                },
                "type": "isWestOf"
            }
            object_properties.append(isEastOf_object_property)
            object_properties.append(isWestOf_object_property)

        area.append(area_dict)
        label_lon += 1
        swLon = ((swLon*forint) + (lineStep*forint)) / forint
        neLon = ((neLon*forint) + (lineStep*forint)) / forint
    label_lat += 1
    swLat = ((swLat*forint) + (lineStep*forint)) / forint
    neLat = ((neLat*forint) + (lineStep*forint)) / forint
    
area_json = json_file_path + "/config_main_area.json"
with open(area_json, 'w') as f:
    json.dump(data, f, indent=4)