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
json_file = os.getenv("PROJECT_PATH") + "/Main/config/json_files/config_main_pmnode.json"
with open(json_file) as f:
    data = json.load(f)

for property in data["pmnodes"]:
    for physical in property["pmnode"]:
        label = physical["property-label"]
        relation = physical["relation-label"]
        data_property = physical["data-property"]
        node = Node(label, **data_property)
        node.add_label("PNode")
        graph.create(node)
        #pmnode-psinkのリレーションを作成 (start)
        hserver = relation["HomeServer"]
        count_psink = graph.run("MATCH (n:Server), (m:VPoint), (l:PSink) WHERE n.Label = \"%s\" AND (n)-[:supports]->(m)-[:isComposedOf]->(l) return count(l)" % hserver)
        result_psink = graph.run("MATCH (n:Server), (m:VPoint), (l:PSink) WHERE n.Label = \"%s\" AND (n)-[:supports]->(m)-[:isComposedOf]->(l) return l" % hserver)
        for record in result_psink:
            #ランダムなPSinkを選べるようにしたい
            pmnode_psink_psink = record["l"]
            pmnode_psink_psink_label = pmnode_psink_psink["Label"]
        rel_pmnode_psink = Relationship(node, "respondsViaDevApi", pmnode_psink_psink)
        rel_psink_pmnode = Relationship(pmnode_psink_psink, "requestsViaDevApi", node)
        graph.create(rel_pmnode_psink)
        graph.create(rel_psink_pmnode)
        #pmnode-psinkのリレーションを作成 (end)
        #pmnode-vpointのリレーションを作成 (start)
        #pnode_id = relation["PNodeID"]
        #config_file = relation["config-file"]
        result_vpoint = graph.run("MATCH (n:PSink), (m:VPoint) WHERE n.Label = \"%s\" AND (n)-[:isVirtualizedWith]->(m) return m" % pmnode_psink_psink_label)
        for record in result_vpoint:
            vpoint_pmnode_vpoint = record["m"]
            vpoint_pmnode_vpoint_label = vpoint_pmnode_vpoint["Label"]
        #rel_pmnode_vpoint = Relationship(node, "isVirtualizedWith", vpoint_pmnode_vpoint)
        #rel_vpoint_pmnode = Relationship(vpoint_pmnode_vpoint, "isComposedOf", node)
        #graph.create(rel_pmnode_vpoint)
        #graph.create(rel_vpoint_pmnode)
        #pmnode-vpointのリレーションを作成 (end)
    for mserver in property["mserver"]:
        label = mserver["property-label"]
        data_property = mserver["data-property"]
        node = Node(label, **data_property)
        node.add_label("Server")
        graph.create(node)
        object_properties = mserver["object-property"]
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
    for virtual in property["vmnodeh"]:
        label = virtual["property-label"]
        data_property = virtual["data-property"]
        node = Node(label, **data_property)
        node.add_label("VNode")
        graph.create(node)
        #vmnodeh-vpointのリレーションを作成 (start)
        rel_vsnode_vpoint = Relationship(node, "requestsViaPrimApi", vpoint_pmnode_vpoint)
        rel_vpoint_vsnode = Relationship(vpoint_pmnode_vpoint, "respondsViaPrimApi", node)
        graph.create(rel_vsnode_vpoint)
        graph.create(rel_vpoint_vsnode)
        #vmnodeh-vpointのリレーションを作成 (end)
        #vmnodeh-serverのリレーションを作成 (start)
        result_server = graph.run("MATCH (n:VPoint), (m:Server) WHERE n.Label = \"%s\" AND (n)-[:isRunningOn]->(m) return m" % vpoint_pmnode_vpoint_label)
        for record in result_server:
            vsnode_server_server = record["m"]
        rel_vsnode_server = Relationship(node, "isRunningOn", vsnode_server_server)
        rel_server_vsnode = Relationship(vsnode_server_server, "supports", node)
        graph.create(rel_vsnode_server)
        graph.create(rel_server_vsnode)
        #vmnodeh-serverのリレーションを作成 (end)
        object_properties = virtual["object-property"]
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
    print("Success: PMNode and VMNodeH Instance")

#indexの付与
def create_index(graph, object, property):
    query = f"CREATE INDEX index_{object}_{property} IF NOT EXISTS FOR (n:{object}) ON (n.{property});"
    graph.run(query)

create_index(graph, "PMNode", "Label")
create_index(graph, "VMNodeH", "Label")
    