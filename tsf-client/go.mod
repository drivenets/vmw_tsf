module github.com/drivenets/vmw_tsf/tsf-client

go 1.16

replace github.com/drivenets/vmw_tsf/tsf-hal => ../tsf-hal

replace github.com/drivenets/vmw_tsf/tsf-twamp => ../tsf-twamp

require (
	github.com/drivenets/vmw_tsf/tsf-hal v0.0.0-20210526133636-9122735a64f8
	github.com/drivenets/vmw_tsf/tsf-twamp v0.0.0-00010101000000-000000000000 // indirect
)
