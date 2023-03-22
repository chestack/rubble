#!/bin/sh

set -x


# 列出所有的NetNS
echo "所有的NetNS："
ip netns list | grep wb

# 遍历每个NetNS，并删除 eth0 和 veth0 两个网卡后再删除该NetNS
for ns in $(ip netns list | grep wb); do
  echo "正在处理NetNS ${ns}..."
  ip netns exec ${ns} ip link delete eth0
  ip netns exec ${ns} ip link delete veth0
  ip netns delete ${ns}
done

# 显示处理结束的信息
echo "处理完毕。"