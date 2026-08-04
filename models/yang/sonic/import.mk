
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

##
# Local sonic yangs that import modules from SONICYANG_IMPORTS must be copied
# after those imports are synced into build/yang/sonic.
#
SONIC_YANG_DEFERRED_NAMES += sonic-port-qos-map.yang sonic-queue.yang
