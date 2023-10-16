import json
from py2neo import Graph, Node, Relationship
import sys
from dotenv import load_dotenv
import os

load_dotenv()

local_url = "bolt://localhost:" + os.getenv("NEO4J_LOCAL_PORT_PYTHON")
local_graph = Graph(local_url, auth=(os.getenv("NEO4J_USERNAME"), os.getenv("NEO4J_LOCAL_PASSWORD")))
local_tx = local_graph.begin()

args = sys.argv
json_file = os.getenv("HOME") + os.getenv("PROJECT_NAME") + "/setup/GraphDB/config/config_main_pmnode.json"
with open(json_file) as f:
    data = json.load(f)

for property in data["pmnodes"]:
    # PMNode Data Property
    pmnode = property["pmnode"]
    label_pmnode = pmnode["property-label"]
    data_property_pmnode = pmnode["data-property"]
    node_pmnode = Node(label_pmnode, **data_property_pmnode)
    node_pmnode.add_label("PNode")
    local_graph.create(node_pmnode)
    
    # VMNode Data Property
    vmnode = property["vmnode"]
    label_vmnode = vmnode["property-label"]
    data_property_vmnode = vmnode["data-property"]
    node_vmnode = Node(label_vmnode, **data_property_vmnode)
    node_vmnode.add_label("VNode")
    local_graph.create(node_vmnode)
    
    # PMNode Object Property
    object_properties_pmnode = pmnode["object-property"]
    for object_property in object_properties_pmnode:
        from_node_label = object_property["from"]["property-label"]
        from_node_property = object_property["from"]["data-property"]
        from_node_value = object_property["from"]["value"]
        to_node_label = object_property["to"]["property-label"]
        to_node_property = object_property["to"]["data-property"]
        to_node_value = object_property["to"]["value"]
        rel_type = object_property["type"]
        from_node = local_graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
        to_node = local_graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
        rel = Relationship(from_node, rel_type, to_node)
        local_graph.create(rel)
    
    # VSNode Object Property
    object_properties_vpoint = vmnode["object-property"]
    for object_property in object_properties_vpoint:
        from_node_label = object_property["from"]["property-label"]
        from_node_property = object_property["from"]["data-property"]
        from_node_value = object_property["from"]["value"]
        to_node_label = object_property["to"]["property-label"]
        to_node_property = object_property["to"]["data-property"]
        to_node_value = object_property["to"]["value"]
        rel_type = object_property["type"]
        from_node = local_graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
        to_node = local_graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
        rel = Relationship(from_node, rel_type, to_node)
        local_graph.create(rel)

try:
    local_graph.commit(local_tx)
except:
    print("Cannot Register Data to Local GraphDB")
else:
    print("Success: PMNode and VMNode Instance in Local GraphDB")
    
#indexの付与
def create_index(graph, object, property):
    query = f"CREATE INDEX index_{object}_{property} IF NOT EXISTS FOR (n:{object}) ON (n.{property});"
    graph.run(query)

create_index(local_graph, "PMNode", "Label")
create_index(local_graph, "VMNode", "Label")