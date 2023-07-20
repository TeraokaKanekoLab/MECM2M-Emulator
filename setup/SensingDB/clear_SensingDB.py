import pymysql
from dotenv import load_dotenv
import os

load_dotenv()

host = "localhost"
user = os.getenv("MYSQL_USERNAME")
password = os.getenv("MYSQL_PASSWORD")
global_db = os.getenv("MYSQL_GLOBAL_DB")
local_db = os.getenv("MYSQL_LOCAL_DB")
table = os.getenv("MYSQL_TABLE")

edge_server_num = int(os.getenv("EDGE_SERVER_NUM"))

# DBに接続
global_connection = pymysql.connect(host=host, user=user, password=password, database=global_db)
global_cursor = global_connection.cursor()
local_connection = pymysql.connect(host=host, user=user, password=password, database=local_db)
local_cursor = local_connection.cursor()

# テーブル内の全データを削除するクエリを実行 (global)
delete_query = f"DELETE FROM {table};"
global_cursor.execute(delete_query)

# テーブルを削除する (global)
drop_table_query = f"DROP TABLE {table}"
global_cursor.execute(drop_table_query)

# テーブル内の全データを削除するクエリを実行し，テーブルを削除する (local)
i = 1
while i <= edge_server_num:
    delete_query = f"DELETE FROM {table}_{i};"
    local_cursor.execute(delete_query)
    drop_table_query = f"DROP TABLE {table}_{i}"
    local_cursor.execute(drop_table_query)
    i += 1

# 変更をコミットして，接続を切断する
global_connection.commit()
local_connection.commit()
global_cursor.close()
local_cursor.close()
global_connection.close()
local_connection.close()

print("Clear Global/Local SensingDB")