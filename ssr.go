package libcore

import (
	"bytes"
	"flag"
	"strconv"

	"github.com/v2fly/v2ray-core/v5/common/buf"
	"github.com/v2fly/v2ray-core/v5/common/net/cnc"
	"github.com/v2fly/v2ray-core/v5/proxy/sip003"
	"github.com/v2fly/v2ray-core/v5/transport/internet"

	"libcore/clash/transport/ssr/obfs"
	"libcore/clash/transport/ssr/protocol"
)

var (
	_ sip003.Plugin         = (*shadowsocksrPlugin)(nil)
	_ sip003.StreamPlugin   = (*shadowsocksrPlugin)(nil)
	_ sip003.ProtocolPlugin = (*shadowsocksrPlugin)(nil)
)

func init() {
	sip003.RegisterPlugin("shadowsocksr", func() sip003.Plugin {
		return new(shadowsocksrPlugin)
	})
}

type shadowsocksrPlugin struct {
	host          string
	port          int
	obfs          string
	obfsParam     string
	protocol      string
	protocolParam string

	o obfs.Obfs
	p protocol.Protocol
}

func (p *shadowsocksrPlugin) Init(_, _, _, _, _ string, _ []string) error {
	panic("Please call InitProtocolPlugin.")
}

func (p *shadowsocksrPlugin) InitStreamPlugin(_, _ string) error {
	panic("Please call InitProtocolPlugin.")
}

func (p *shadowsocksrPlugin) InitProtocolPlugin(remoteHost string, remotePort string, pluginArgs []string, key []byte, ivSize int) error {
	fs := flag.NewFlagSet("shadowsocksr", flag.ContinueOnError)
	fs.StringVar(&p.obfs, "obfs", "origin", "")
	fs.StringVar(&p.obfsParam, "obfs-param", "", "")
	fs.StringVar(&p.protocol, "protocol", "origin", "")
	fs.StringVar(&p.protocolParam, "protocol-param", "", "")
	if err := fs.Parse(pluginArgs); err != nil {
		return newError("shadowsocksr: failed to parse args").Base(err)
	}
	p.host = remoteHost
	p.port, _ = strconv.Atoi(remotePort)

	obfs, obfsOverhead, err := obfs.PickObfs(p.obfs, &obfs.Base{
		Host:   p.host,
		Port:   p.port,
		Key:    key,
		IVSize: ivSize,
		Param:  p.obfsParam,
	})
	if err != nil {
		return newError("failed to create ssr obfs").Base(err)
	}

	protocol, err := protocol.PickProtocol(p.protocol, &protocol.Base{
		Key:      key,
		Overhead: obfsOverhead,
		Param:    p.protocolParam,
	})
	if err != nil {
		return newError("failed to create ssr protocol").Base(err)
	}

	p.o = obfs
	p.p = protocol

	return nil
}

func (p *shadowsocksrPlugin) Close() error {
	return nil
}

func (p *shadowsocksrPlugin) StreamConn(conn internet.Connection) internet.Connection {
	return p.o.StreamConn(conn)
}

func (p *shadowsocksrPlugin) ProtocolConn(conn *sip003.ProtocolConn, iv []byte) {
	upstream := cnc.NewConnection(cnc.ConnectionOutputMulti(conn), cnc.ConnectionInputMulti(conn))
	downstream := p.p.StreamConn(upstream, iv)
	if upstream == downstream {
		conn.ProtocolReader = conn
		conn.ProtocolWriter = conn
	} else {
		conn.ProtocolReader = buf.NewReader(downstream)
		conn.ProtocolWriter = buf.NewWriter(downstream)
	}
}

func (p *shadowsocksrPlugin) EncodePacket(buffer *buf.Buffer, ivLen int32) (*buf.Buffer, error) {
	defer buffer.Release()
	packet := &bytes.Buffer{}
	err := p.p.EncodePacket(packet, buffer.BytesFrom(ivLen))
	if err != nil {
		return nil, err
	}
	if ivLen > 0 {
		newBuffer := buf.New()
		newBuffer.Write(buffer.BytesTo(ivLen))
		newBuffer.Write(packet.Bytes())
		return newBuffer, nil
	} else {
		return buf.FromBytes(packet.Bytes()), nil
	}
}

func (p *shadowsocksrPlugin) DecodePacket(buffer *buf.Buffer) (*buf.Buffer, error) {
	defer buffer.Release()
	packet, err := p.p.DecodePacket(buffer.Bytes())
	if err != nil {
		return nil, err
	}
	newBuffer := buf.New()
	newBuffer.Write(packet)
	newBuffer.Endpoint = buffer.Endpoint
	return newBuffer, nil
}
