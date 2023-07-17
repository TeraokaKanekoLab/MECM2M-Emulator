# !/bin/bash

# RTTなどを表現するためのリンクを表すソケットアドレスを生成するためのスクリプト
printf "\e[1;31m \n *** BUILD SOCKET ADDRESS *** \e[0m"

# ホーム下に用意した.envファイルを読み込む
source $HOME/.env
DIR="/setup/LinkProcess"

printf "\e[1;31m \n1-1. BUILD SOCKET ADDRESS FOR INTERNET \e[0m"
# インターネットを表すリンク用のソケットアドレスを生成
python3 ${PROJECT_PATH}${DIR}/create_internet_link_process.py

printf "\e[1;31m \n1-1. BUILD SOCKET ADDRESS FOR CLOSED NETWORK \e[0m"
# 閉域網を表すリンク用のソケットアドレスを生成
python3 ${PROJECT_PATH}${DIR}/create_closed_network_link_process.py

printf "\n"