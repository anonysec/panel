package templates

// defaultTemplates contains embedded default config templates for each protocol.
var defaultTemplates = map[string]string{
	"openvpn": `port {{.Port}}
proto {{.Protocol}}
dev tun
{{- if .ServerNet}}
server {{.ServerNet}} {{.ServerMask}}
{{- end}}
{{- if .NetworkIPv6}}
server-ipv6 {{.NetworkIPv6}}
{{- end}}
push "dhcp-option DNS {{.DNS1}}"
push "dhcp-option DNS {{.DNS2}}"
{{- if .DNS1v6}}
push "dhcp-option DNS6 {{.DNS1v6}}"
{{- end}}
push "redirect-gateway def1 bypass-dhcp"
keepalive 10 120
data-ciphers AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305
data-ciphers-fallback AES-256-GCM
auth SHA256
tls-crypt /etc/openvpn/server/tc.key
ca /etc/openvpn/server/ca.crt
cert /etc/openvpn/server/server.crt
key /etc/openvpn/server/server.key
dh none
persist-key
persist-tun
status /run/openvpn-status.log
verb 3
`,

	"wireguard": `[Interface]
Address = {{.Network}}
ListenPort = {{.Port}}
PrivateKey = SERVER_PRIVATE_KEY_PLACEHOLDER
{{- if .DNS1}}
DNS = {{.DNS1}}{{if .DNS2}}, {{.DNS2}}{{end}}
{{- end}}
`,

	"strongswan": `# StrongSwan IKEv2 configuration
config setup
    charondebug="ike 1, knl 1, cfg 0"

conn ikev2-vpn
    auto=add
    compress=no
    type=tunnel
    keyexchange=ikev2
    ike=aes256-sha256-modp2048!
    esp=aes256-sha256!
    dpdaction=clear
    dpddelay=300s
    rekey=no
    left=%any
    leftid={{.ServerIP}}
    leftcert=server-cert.pem
    leftsendcert=always
    leftsubnet=0.0.0.0/0
    right=%any
    rightid=%any
    rightauth=eap-mschapv2
    rightsourceip={{.Network}}
    rightdns={{.DNS1}},{{.DNS2}}
    eap_identity=%identity
`,

	"xl2tpd": `[global]
port = 1701

[lns default]
ip range = {{.Network}}
local ip = {{.ServerIP}}
require chap = yes
refuse pap = yes
require authentication = yes
name = l2tpd
pppoptfile = /etc/ppp/options.xl2tpd
length bit = yes
`,
}
