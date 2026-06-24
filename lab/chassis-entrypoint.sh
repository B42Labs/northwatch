#!/usr/bin/env bash
# Entrypoint for the Northwatch lab chassis (hypervisor) image.
#
# Starts Open vSwitch (userspace datapath) and ovn-controller, wires the
# Open_vSwitch external_ids so the chassis registers against the central SB,
# then stays in the foreground tailing logs.
#
# The userspace datapath (datapath_type=netdev) is deliberate: the lab only
# needs control-plane state (Chassis registration, Port_Binding, Encap rows),
# never real packet forwarding, so we avoid any dependency on the host
# `openvswitch` kernel module and the lab runs on any Linux Docker host.

set -euo pipefail

log() { printf '[chassis] %s\n' "$*" >&2; }

# Stable chassis identity. Falls back to the container hostname so the image
# still works when launched outside the canned topology.
CHASSIS_NAME="${CHASSIS_NAME:-$(hostname -s)}"
OVN_SB_REMOTE="${OVN_SB_REMOTE:-tcp:central:6642}"
# Encap IP must be unique per chassis. containerlab puts every node on the
# management network as eth0, so its address gives each chassis a distinct
# geneve endpoint (even though no real traffic flows over the tunnels).
ENCAP_IP="${ENCAP_IP:-$(ip -o -4 addr show eth0 | awk '{print $4}' | cut -d/ -f1)}"
BRIDGE_MAPPING="${BRIDGE_MAPPING:-physnet1:br-ex}"
DATAPATH_TYPE="${DATAPATH_TYPE:-netdev}"

start_ovs() {
    mkdir -p /var/run/openvswitch /var/log/openvswitch /etc/openvswitch

    if [ "${DATAPATH_TYPE}" = "system" ]; then
        # Opt-in kernel datapath. Needs the `openvswitch` kernel module, which is
        # NOT in container-optimised kernels like Docker Desktop's LinuxKit — only
        # in a full Linux host kernel (with /lib/modules bind-mounted in). With the
        # kernel datapath, geneve tunnels carry real traffic, BFD between chassis
        # converges, and multi-member HA_Chassis_Group gateways actually bind.
        log "starting Open vSwitch (system/kernel datapath)"
        if ! modprobe openvswitch 2>/dev/null; then
            echo "ERROR: 'openvswitch' kernel module not available." >&2
            echo "  The system datapath needs a Linux host whose kernel has the module" >&2
            echo "  and /lib/modules bind-mounted in. Docker Desktop's LinuxKit kernel" >&2
            echo "  does not ship it — use the default userspace datapath there." >&2
            exit 1
        fi
        # The standard ovs-ctl start loads the module and launches ovsdb-server +
        # ovs-vswitchd with the kernel datapath.
        /usr/share/openvswitch/scripts/ovs-ctl --system-id="${CHASSIS_NAME}" start
    else
        # Default userspace (netdev) datapath. We pass --no-ovs-vswitchd because
        # ovs-ctl's vswitchd startup tries to insert the `openvswitch` kernel
        # module (absent in container kernels), which makes `ovs-ctl start` fail.
        # We then start ovs-vswitchd ourselves; it never touches the kernel module
        # as long as every bridge uses datapath_type=netdev.
        log "starting Open vSwitch (netdev/userspace datapath, no kernel module)"
        /usr/share/openvswitch/scripts/ovs-ctl --system-id="${CHASSIS_NAME}" --no-ovs-vswitchd start
        ovs-vsctl --no-wait init
        ovs-vswitchd --pidfile --detach --log-file=/var/log/openvswitch/ovs-vswitchd.log \
            unix:/var/run/openvswitch/db.sock
    fi

    for _ in $(seq 1 30); do
        if ovs-vsctl show >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    echo "ovs-vsctl did not respond after start" >&2
    exit 1
}

configure_ovs() {
    log "configuring Open_vSwitch external_ids for ovn-controller (chassis ${CHASSIS_NAME})"
    ovs-vsctl set Open_vSwitch . \
        external_ids:ovn-remote="${OVN_SB_REMOTE}" \
        external_ids:ovn-encap-type=geneve \
        external_ids:ovn-encap-ip="${ENCAP_IP}" \
        external_ids:system-id="${CHASSIS_NAME}" \
        external_ids:hostname="${CHASSIS_NAME}" \
        external_ids:ovn-bridge-mappings="${BRIDGE_MAPPING}"

    # Pre-create the integration bridge with the chosen datapath BEFORE
    # ovn-controller starts; it will adopt the existing bridge. br-ex backs the
    # physnet1 bridge mapping so localnet ports show up.
    log "ensuring br-int and br-ex with datapath_type=${DATAPATH_TYPE}"
    ovs-vsctl --may-exist add-br br-int -- set bridge br-int datapath_type="${DATAPATH_TYPE}"
    ovs-vsctl --may-exist add-br br-ex  -- set bridge br-ex  datapath_type="${DATAPATH_TYPE}"
    ip link set br-int up || true
    ip link set br-ex up || true
}

start_ovn_controller() {
    log "starting ovn-controller"
    /usr/share/ovn/scripts/ovn-ctl start_controller
    for _ in $(seq 1 30); do
        if ovs-vsctl br-exists br-int 2>/dev/null && pgrep -x ovn-controller >/dev/null; then
            return 0
        fi
        sleep 1
    done
    echo "ovn-controller did not come up" >&2
    exit 1
}

main() {
    start_ovs
    configure_ovs
    start_ovn_controller
    log "chassis ${CHASSIS_NAME} ready (encap ${ENCAP_IP}); registered against ${OVN_SB_REMOTE}"

    # No agent to exec into — keep the container alive and surface logs.
    exec tail -F /var/log/openvswitch/ovs-vswitchd.log \
                  /var/log/ovn/ovn-controller.log 2>/dev/null
}

main "$@"
