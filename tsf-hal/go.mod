module github.com/drivenets/vmw_tsf/tsf-hal

go 1.16

replace github.com/drivenets/vmw_tsf/tsf-twamp => ../tsf-twamp

require (
	github.com/Juniper/go-netconf v0.1.1
	github.com/cloudflare/goflow/v3 v3.4.2
	github.com/drivenets/vmw_tsf/tsf-twamp v0.0.0-00010101000000-000000000000
	github.com/openconfig/gnmi v0.0.0-20210525213403-320426956c8a
	github.com/openconfig/ygot v0.10.10
	github.com/sirupsen/logrus v1.8.1
	github.com/ziutek/telnet v0.0.0-20180329124119-c3b780dc415b // indirect
	golang.org/x/crypto v0.0.0-20200302210943-78000ba7a073
	google.golang.org/grpc v1.38.0
)
