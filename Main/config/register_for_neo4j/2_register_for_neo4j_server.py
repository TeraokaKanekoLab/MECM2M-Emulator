import json
from py2neo import Graph, Node, Relationship
import sys
from dotenv import load_dotenv
import os

load_dotenv()
url = "bolt://localhost:" + os.getenv("NEO4J_PORT")
graph = Graph(url, auth=(os.getenv("NEO4J_USERNAME"), os.getenv("NEO4J_PASSWORD")))

tx = graph.begin()

args = sys.argv
json_file = os.getenv("HOME") + os.getenv("PROJECT_PATH") + "/Main/config/json_files/config_main_server.json"
with open(json_file) as f:
    data = json.load(f)

areas = data["servers"]
for property in areas["server"]:
    label = property["property-label"]
    data_property = property["data-property"]
    node = Node(label, **data_property)
    graph.create(node)
    object_properties = property["object-property"]
    for object_property in object_properties:
        from_node_label = object_property["from"]["property-label"]
        from_node_property = object_property["from"]["data-property"]
        from_node_value = object_property["from"]["value"]
        to_node_label = object_property["to"]["property-label"]
        to_node_property = object_property["to"]["data-property"]
        to_node_value = object_property["to"]["value"]
        rel_type = object_property["type"]
        from_node = graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
        to_node = graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
        rel = Relationship(from_node, rel_type, to_node)
        graph.create(rel)

try:
    graph.commit(tx)
except:
    print("Cannot Register Data to Neo4j")
else:
    print("Success: Server Instance")

#indexの付与
def create_index(graph, object, property):
    query = f"CREATE INDEX index_{object}_{property} IF NOT EXISTS FOR (n:{object}) ON (n.{property});"
    graph.run(query)

create_index(graph, "Server", "Label")
