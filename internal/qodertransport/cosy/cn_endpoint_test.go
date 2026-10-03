package cosy

import "testing"

func TestEndpointCNChatURL(t *testing.T) {
	u, err := endpointURL(EndpointCN)
	if err != nil {
		t.Fatalf("endpointURL(EndpointCN): %v", err)
	}
	const want = "https://gateway.qoder.com.cn/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
	if u != want {
		t.Errorf("endpointURL(EndpointCN)=\n%s\nwant\n%s", u, want)
	}
}

func TestCatalogURLCN(t *testing.T) {
	u, err := catalogURL(EndpointCN)
	if err != nil {
		t.Fatalf("catalogURL(EndpointCN): %v", err)
	}
	const want = "https://gateway.qoder.com.cn/algo/api/v2/model/list"
	if u != want {
		t.Errorf("catalogURL(EndpointCN)=%q, want %q", u, want)
	}
}

func TestNewTransportAcceptsEndpointCN(t *testing.T) {
	tr, err := NewTransport(Config{Endpoint: EndpointCN, BaseURL: "http://127.0.0.1:0", AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport(EndpointCN): %v", err)
	}
	if tr == nil {
		t.Fatal("NewTransport(EndpointCN)=nil")
	}
}
