
##
# SONICYANG_IMPORTS is the list of sonic yang files to be copied
# from SONICYANG_IMPORT_DIR. Only the file basenames (like sonic-sflow.yang)
# or glob patterns of basenames (like sonic-telemetry*.yang) can be specified.
# Other sonic yangs referred by these will also be copied.
#
SONICYANG_IMPORTS += sonic-sflow.yang
SONICYANG_IMPORTS += sonic-interface.yang
SONICYANG_IMPORTS += sonic-port.yang
SONICYANG_IMPORTS += sonic-vlan.yang
SONICYANG_IMPORTS += sonic-mclag.yang
SONICYANG_IMPORTS += sonic-types.yang
SONICYANG_IMPORTS += sonic-vrf.yang
SONICYANG_IMPORTS += sonic-lldp.yang
SONICYANG_IMPORTS += sonic-portchannel.yang
SONICYANG_IMPORTS += sonic-vrrp.yang
SONICYANG_IMPORTS += sonic-spanning-tree.yang
# sonic-gnmi provides the GNMI table (port/dscp/...). It is required so CVL can
# validate translib writes to GNMI|gnmi driven by the OpenConfig grpc-server
# config path; without it CVL rejects the write ("Invalid table or key for
# GNMI|gnmi"). Its sonic-mgmt_vrf import is pulled in transitively.
SONICYANG_IMPORTS += sonic-gnmi.yang
# sonic-storm-control provides the PORT_STORM_CONTROL table. Required so CVL can
# validate translib writes driven by the openconfig-storm-control-ext augment;
# without it CVL rejects the write ("Invalid table or key for
# PORT_STORM_CONTROL|<ifname>|<storm_type>"). Imports sonic-port, already listed.
SONICYANG_IMPORTS += sonic-storm-control.yang
# sonic-auto_techsupport provides the AUTO_TECHSUPPORT table. Required so CVL can
# validate translib writes to AUTO_TECHSUPPORT|GLOBAL:max_core_limit and
# max_techsupport_limit driven by the openconfig-system-ext core-file-cleanup
# and tech-support-cleanup augments. Imports sonic-types, already listed.
SONICYANG_IMPORTS += sonic-auto_techsupport.yang

# BGP global OC mapping uses CONFIG_DB:BGP_GLOBALS|<vrf>.
# Import BGP and routing-policy SONiC YANGs so CVL schema contains
# table/key metadata for all mapped OC paths.
SONICYANG_IMPORTS += sonic-bgp-global.yang
SONICYANG_IMPORTS += sonic-bgp-device-global.yang
SONICYANG_IMPORTS += sonic-bgp-neighbor.yang
SONICYANG_IMPORTS += sonic-bgp-peergroup.yang
SONICYANG_IMPORTS += sonic-route-common.yang
SONICYANG_IMPORTS += sonic-route-map.yang
SONICYANG_IMPORTS += sonic-routing-policy-sets.yang
SONICYANG_IMPORTS += sonic-loopback-interface.yang

# Static route OC mapping uses CONFIG_DB:STATIC_ROUTE|<vrf>|<prefix>.
SONICYANG_IMPORTS += sonic-static-route.yang

# DHCPv6 relay OC mapping uses CONFIG_DB:DHCP_RELAY|<vlan>.
SONICYANG_IMPORTS += sonic-dhcpv6-relay.yang

# DHCPv4 relay OC mapping uses CONFIG_DB:DHCPV4_RELAY|<vlan> or VLAN|<vlan>.
SONICYANG_IMPORTS += sonic-dhcpv4-relay.yang

# sonic-syslog is the ConfigDB SYSLOG_SERVER / SYSLOG_CONFIG schema (TLS
# protocol, tls_profile_id, VRF, filters, FEATURE rate-limits). Import it so
# CVL validates translib writes driven by OpenConfig logging the same way
# sonic-yang-mgmt validates config CLI. TLS apply-state leaves stay in the
# local sonic-syslog-state augment (see SONIC_YANG_DEFERRED_NAMES).
SONICYANG_IMPORTS += sonic-syslog.yang

##
# Local sonic yangs that import modules from SONICYANG_IMPORTS must be copied
# after those imports are synced into build/yang/sonic.
#
SONIC_YANG_DEFERRED_NAMES += sonic-port-qos-map.yang sonic-queue.yang
# Augments imported sonic-syslog with STATE_DB TLS apply-state leaves.
SONIC_YANG_DEFERRED_NAMES += sonic-syslog-state.yang
