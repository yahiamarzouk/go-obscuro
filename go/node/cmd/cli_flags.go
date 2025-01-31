package main

// Flag names.
const (
	nodeNameFlag               = "node_name"
	nodeTypeFlag               = "node_type"
	isGenesisFlag              = "is_genesis"
	hostIDFlag                 = "host_id"
	isSGXEnabledFlag           = "is_sgx_enabled"
	enclaveDockerImageFlag     = "enclave_docker_image"
	hostDockerImageFlag        = "host_docker_image"
	l1HostFlag                 = "l1_host"
	l1WSPortFlag               = "l1_ws_port"
	hostHTTPPortFlag           = "host_http_port"
	hostWSPortFlag             = "host_ws_port"
	hostP2PPortFlag            = "host_p2p_port"
	hostP2PHostFlag            = "host_p2p_host"
	hostP2PPublicAddrFlag      = "host_public_p2p_addr"
	enclaveHTTPPortFlag        = "enclave_http_port"
	enclaveWSPortFlag          = "enclave_WS_port"
	privateKeyFlag             = "private_key"
	sequencerIDFlag            = "sequencer_id"
	managementContractAddrFlag = "management_contract_addr"
	messageBusContractAddrFlag = "message_bus_contract_addr"
	pccsAddrFlag               = "pccs_addr"
	edgelessDBImageFlag        = "edgeless_db_image"
)

// Returns a map of the flag usages.
// While we could just use constants instead of a map, this approach allows us to test that all the expected flags are defined.
func getFlagUsageMap() map[string]string {
	return map[string]string{
		nodeNameFlag:               "Specifies the node base name",
		nodeTypeFlag:               "The node's type (e.g. sequencer, validator)",
		isGenesisFlag:              "Wether the node is the genesis node of the network",
		hostIDFlag:                 "The 20 bytes of the address of the Obscuro host this enclave serves",
		isSGXEnabledFlag:           "Whether the it should run on an SGX is enabled CPU",
		enclaveDockerImageFlag:     "Docker image for the enclave",
		hostDockerImageFlag:        "Docker image for the host",
		l1HostFlag:                 "Layer 1 network host addr",
		l1WSPortFlag:               "Layer 1 network WebSocket port",
		hostP2PPortFlag:            "Hosts p2p bound port",
		hostP2PPublicAddrFlag:      "Hosts public p2p host.",
		hostP2PHostFlag:            "Hosts p2p bound addr",
		enclaveHTTPPortFlag:        "Enclave's http bound port",
		enclaveWSPortFlag:          "Enclave's WS bound port",
		privateKeyFlag:             "L1 and L2 private key used in the node",
		sequencerIDFlag:            "The 20 bytes of the address of the sequencer for this network",
		managementContractAddrFlag: "The management contract address on the L1",
		messageBusContractAddrFlag: "The address of the L1 message bus contract owned by the management contract.",
		pccsAddrFlag:               "Sets the PCCS address",
		edgelessDBImageFlag:        "Sets the edgelessdb image",
		hostHTTPPortFlag:           "Host HTTPs bound port",
		hostWSPortFlag:             "Host WebSocket bound port",
	}
}
