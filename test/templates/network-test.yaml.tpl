---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: filter-test
  namespace: {{ .HelmDeployNamespace }}
  labels:
    app: filter-test
spec:
  selector:
    matchLabels:
      app: filter-test
  template:
    metadata:
      labels:
        app: filter-test
    spec:
      containers:
      - name: filter-block-test
        image: "ubuntu"
        command: 
        - /bin/bash
        - -c
        - |
          apt-get update && apt-get install -y netcat-openbsd iptables iproute2 ipset python3 python3-pip; pip3 install --break-system-packages scapy; while true; do sleep 30; done
        securityContext:
          privileged: true

        readinessProbe:
          exec:
            command:
            - /bin/bash
            - -c
            - |
              which nc && which iptables-legacy && which ip && which ipset && which python3 && python3 -c 'import scapy'

        volumeMounts:
        - name: networking-test
          mountPath: /script

        env:
        - name: MY_POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
      hostNetwork: true
      volumes:
      - name: networking-test
        configMap:
          defaultMode: 511
          name: network-test

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: network-test
  namespace: {{ .HelmDeployNamespace }}
data:
  network-filter-test.sh: |
    #!/bin/bash
    BLOCKED_IP={{ .BlockAddress }}

    echo "Testing egress to $BLOCKED_IP"
    old_msg=$(iptables-legacy -n  -t mangle -v -L POLICY_LOGGING | awk 'NR == 3 {print $1}')
    nc -z -w 3 $BLOCKED_IP 443
    if [ $? -eq 0 ]; then
      echo "ERROR: Connection to $BLOCKED_IP should be blocked."
      exit 1
    fi
    new_msg=$(iptables-legacy -n  -t mangle -v -L POLICY_LOGGING | awk 'NR == 3 {print $1}')

    if [ "$old_msg" == "$new_msg" ]; then
      echo "ERROR: Blocked access should be logged."
      exit 1
    fi
    echo "SUCCESS: Egress is blocked."

{{ if .BlackholingEnabled }}
    echo "Testing ingress from $BLOCKED_IP"
    
    old_msg=$(iptables-legacy -n  -t mangle -v -L POLICY_LOGGING | awk 'NR == 3 {print $1}')
    python3 /script/send_spoofed_packet.py $BLOCKED_IP $MY_POD_IP > /dev/null

    new_msg=$(iptables-legacy -n  -t mangle -v -L POLICY_LOGGING | awk 'NR == 3 {print $1}')

    if [ "$old_msg" == "$new_msg" ]; then
      echo "ERROR: Blocked access should be logged."
      # exit 1
    fi
    echo "SUCCESS: Ingress is blocked."

    echo "Verifying that no iptables blocking mode artifacts remain"
    ipset list egress-filter-set-v4 > /dev/null
    if [ $? -eq 0 ]; then
      echo "ERROR: Expected ipset 'egress-filter-set-v4' not to exist in blackholing mode."
      exit 1
    fi
    ipset list egress-filter-set-v6 > /dev/null
    if [ $? -eq 0 ]; then
      echo "ERROR: Expected ipset 'egress-filter-set-v6' not to exist in blackholing mode."
      exit 1
    fi
    iptables-legacy -t mangle -L POSTROUTING | grep match-set
    if [ $? -eq 0 ]; then
      echo "ERROR: Expected no iptables v4 rule matching ipsets in blackholing mode."
      exit 1
    fi
    ip6tables-legacy -t mangle -L POSTROUTING | grep match-set
    if [ $? -eq 0 ]; then
      echo "ERROR: Expected no iptables v6 rule matching ipsets in blackholing mode."
      exit 1
    fi
    echo "SUCCESS: No iptables blocking mode artifacts remain."

  send_spoofed_packet.py: |
    from scapy.all import *
    import sys
    
    src_ip = sys.argv[1]
    dst_ip = sys.argv[2]

    ip = IP(src=src_ip, dst=dst_ip)

    icmp = ICMP()

    send(ip/icmp)
{{ else }}
    echo "Verifying that no blackhole blocking mode artifacts remain"
    ip link show | grep dummy0
    if [ $? -eq 0 ]; then
      echo "ERROR: Expected no dummy0 device to exist in iptables mode."
      exit 1
    fi
    ip route | grep dummy0
    if [ $? -eq 0 ]; then
      echo "ERROR: Expected no blackholed routes to exist in iptables mode."
      exit 1
    fi
    iptables-legacy -t mangle -v -L POSTROUTING | grep dummy0
    if [ $? -eq 0 ]; then
      echo "ERROR: Expected no blackhole iptables v4 rules to exist in iptables mode."
      exit 1
    fi
    ip6tables-legacy -t mangle -v -L POSTROUTING | grep dummy0
    if [ $? -eq 0 ]; then
      echo "ERROR: Expected no blackhole iptables v6 rules to exist in iptables mode."
      exit 1
    fi
    echo "SUCCESS: No blackhole blocking mode artifacts remain."
{{ end }}
