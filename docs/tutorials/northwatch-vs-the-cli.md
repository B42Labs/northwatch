# Northwatch vs. the command line

Everything Northwatch shows you can also be found with `ovn-nbctl`, `ovn-sbctl`,
`ovn-trace`, `ovs-vsctl`, `ovs-ofctl` and `ovs-appctl`. The upstream
[OVN OpenStack tutorial](https://docs.ovn.org/en/stable/tutorials/ovn-openstack.html)
teaches exactly that workflow, and it is worth knowing. It also shows the cost:
almost every answer is a *chain* of commands in which you read a UUID, a MAC, a
tunnel key or an OpenFlow port number out of one output and paste it into the
next command, often on a different host.

This tutorial walks through eight everyday questions twice: first with the
command line, with the real output, then with Northwatch. Each scenario ends
with a tally so you can see what the correlation work costs when you do it by
hand.

All output on this page was captured from the [local lab](/how-to/run-the-local-lab).
UUIDs, chassis placement and OpenFlow port numbers will differ in your lab, which
is rather the point: on the command line you have to look them up again every
time.

## Before you start

Bring the lab up, bind the ports, and start Northwatch with per-chassis OVS
visibility enabled (the lab exports each chassis's OVSDB on
`127.0.0.1:6650`–`6652`; the mapping lives in `lab/ovs-mgmt.json`):

```bash
make lab-compose        # lab up + seed
make lab-bind           # bind the VIFs onto the chassis
make build
./bin/northwatch \
  --ovn-nb-addr tcp:127.0.0.1:6641 --ovn-sb-addr tcp:127.0.0.1:6642 \
  --ovs-mgmt-addr-file lab/ovs-mgmt.json
```

Scenario 8 (impact analysis) additionally needs the write API, because impact
analysis is part of it. Add `--write-enabled --api-tokens demo=<16+ characters>`
when you get there.

For the command-line side, define three helpers so the commands read like they
would on a real deployment. In production, `nbctl`/`sbctl` run on a control-plane
node and `on <chassis>` is an SSH session to a hypervisor:

```bash
nbctl() { docker exec clab-nw-lab-central ovn-nbctl "$@"; }
sbctl() { docker exec clab-nw-lab-central ovn-sbctl "$@"; }
on()    { local c=$1; shift; docker exec "clab-nw-lab-$c" "$@"; }
```

## Scenario 1: Where does this IP live? {#scenario-1}

*A user reports a problem with `10.12.0.13`. Which logical port is that, which
hypervisor hosts it, and which OVS interface is it plugged into?*

This is a mixed OVN + OVS question: the answer starts in the Northbound
database, crosses the Southbound database, and ends in the `Open_vSwitch`
database of one specific chassis.

### With the command line

**1.** Find the logical switch port. There is no index on IP addresses, so list
the ports and `grep`:

```console
$ nbctl --columns=_uuid,name,addresses,up find Logical_Switch_Port | grep -B2 -A1 '10.12.0.13"'
_uuid               : 719b170a-e232-49bb-abb4-eac51a530266
name                : nw-ls-003-vif-004
addresses           : ["02:00:00:30:03:04 10.12.0.13"]
up                  : true
```

**2.** Take the port **name** to the Southbound database to find the binding:

```console
$ sbctl --columns=_uuid,logical_port,chassis,datapath,tunnel_key,up find Port_Binding logical_port=nw-ls-003-vif-004
_uuid               : 7b2c1575-e32b-47b1-b5f7-d5bd9b2363cb
logical_port        : nw-ls-003-vif-004
chassis             : 31e24ad7-8e28-4a03-8082-01eda02a09e4
datapath            : 735af730-92aa-45f7-ad86-a6143058653c
tunnel_key          : 4
up                  : true
```

**3.** The chassis is a UUID. Copy the **chassis UUID** to resolve it:

```console
$ sbctl --columns=name,hostname,encaps list Chassis 31e24ad7-8e28-4a03-8082-01eda02a09e4
name                : chassis-3
hostname            : chassis-3
encaps              : [99f3cc77-99e9-44a4-8c6e-13b579fe4b56]
```

**4.** The tunnel endpoint is yet another row. Use the **chassis name**:

```console
$ sbctl --columns=type,ip find Encap chassis_name=chassis-3
type                : geneve
ip                  : "172.19.0.3"
```

**5.** Now change hosts. Log in to `chassis-3` and look for the interface whose
`iface-id` matches the **port name** from step 1:

```console
$ on chassis-3 ovs-vsctl --columns=name,ofport,external_ids,admin_state,link_state,error \
    find Interface external_ids:iface-id=nw-ls-003-vif-004
name                : nw502c4065
ofport              : 2
external_ids        : {iface-id=nw-ls-003-vif-004, ovn-installed="true", ovn-installed-ts="1789894593224"}
admin_state         : down
link_state          : down
error               : []
```

Did you notice that Southbound says `up : true` while the OVS interface says
`link_state : down`? On the command line that contradiction sits in two
different outputs on two different hosts, and nothing points it out.

### With Northwatch

Type `10.12.0.13` into the search field. Omnisearch detects an IPv4 address and
searches both databases at once:

![Omnisearch results for an IP address](/img/northwatch-vs-cli/search-ip.jpg)

Click the Logical Switch Port. The correlated view shows the whole binding
chain — Northbound port, Southbound port binding with its tunnel key, chassis,
and datapath binding — with names instead of UUIDs:

![Correlated logical switch port with binding chain](/img/northwatch-vs-cli/correlated-port.jpg)

For the OVS side, open **OVS Visibility → chassis-3** and pick the interface.
Northwatch correlates the OVS row back to OVN and flags the contradiction from
step 5 as **drift**:

![OVS interface detail with OVN correlation and drift](/img/northwatch-vs-cli/ovs-interface.jpg)

The same over the API:

```bash
curl -s 'http://localhost:8080/api/v1/search?q=10.12.0.13'
curl -s http://localhost:8080/api/v1/correlated/logical-switch-ports/719b170a-e232-49bb-abb4-eac51a530266
curl -s http://localhost:8080/api/v1/ovs/chassis-3/interface/4528f41a-3972-4f4a-968c-1f3ea877f782/correlation
```

```json
{
  "iface_id": "nw-ls-003-vif-004",
  "bound": true,
  "binding": {
    "logical_port": "nw-ls-003-vif-004",
    "up": true,
    "chassis": "chassis-3",
    "bound_here": true,
    "datapath": "nw-ls-003"
  },
  "drift": ["SB reports the port up but the OVS interface link_state is down"]
}
```

| | Command line | Northwatch |
|---|---|---|
| Steps | 5 commands | 1 search, 2 clicks |
| Hosts | 2 (control plane + hypervisor) | 1 browser tab |
| Values copied by hand | 3 (port name, chassis UUID, chassis name) | 0 |
| Contradiction between SB and OVS | yours to spot | flagged as drift |

## Scenario 2: Why is this port down? {#scenario-2}

*`nw-ls-002-vif-003` has no connectivity.* To reproduce the failure, pull the
port's OVS interface off its chassis:

```bash
on chassis-1 ovs-vsctl del-port br-int \
  "$(on chassis-1 ovs-vsctl --bare --columns=name find Interface external_ids:iface-id=nw-ls-002-vif-003)"
```

(Use whichever chassis hosts the port in your lab; scenario 1 shows how to find
it. `make lab-unbind && make lab-bind` repairs it afterwards; `lab-bind` alone
skips ports it has already bound once.)

### With the command line

**1.** Is the port up?

```console
$ nbctl lsp-get-up nw-ls-002-vif-003
down
```

**2.** Is it disabled, or a special type?

```console
$ nbctl --columns=name,type,enabled,up,options list Logical_Switch_Port nw-ls-002-vif-003
name                : nw-ls-002-vif-003
type                : ""
enabled             : true
up                  : false
options             : {}
```

**3.** Is it bound anywhere?

```console
$ sbctl --columns=logical_port,chassis,requested_chassis,up find Port_Binding logical_port=nw-ls-002-vif-003
logical_port        : nw-ls-002-vif-003
chassis             : []
requested_chassis   : []
up                  : false
```

**4.** Not bound, and no `requested_chassis` to tell you where it *should* be.
So ask every hypervisor whether it has an interface for that port:

```console
$ for c in chassis-1 chassis-2 chassis-3; do
    echo "$c: $(on $c ovs-vsctl --bare --columns=name,ofport,error find Interface external_ids:iface-id=nw-ls-002-vif-003)"
  done
chassis-1:
chassis-2:
chassis-3:
```

Three hosts in the lab. On a real cloud this loop runs over every compute node.

**5.** Nobody has it. To learn where it used to be, `grep` the
`ovn-controller` logs, again per host:

```console
$ on chassis-1 grep nw-ls-002-vif-003 /var/log/ovn/ovn-controller.log | tail -2
2026-09-20T09:00:14.188Z|00090|binding|INFO|Releasing lport nw-ls-002-vif-003 from this chassis (sb_readonly=0)
2026-09-20T09:00:14.188Z|00091|binding|INFO|Setting lport nw-ls-002-vif-003 down in Southbound
```

### With Northwatch

Open **Debug → Port Diagnostics**. Every logical port is checked continuously;
the broken one sorts to the top and the failing check names the cause:

![Port Diagnostics with one failing port expanded](/img/northwatch-vs-cli/port-diagnostics.jpg)

The same condition also raises the built-in "VIF port not bound to any chassis"
alert, so you would normally hear about it before a user does. **History &
Events → Events** holds the `Port_Binding` change with its timestamp, which
replaces the log `grep` in step 5.

```bash
curl -s http://localhost:8080/api/v1/debug/port-diagnostics/c0b47a4b-14cf-404e-8846-3131899c818d
```

```text
error
  healthy binding_status   - Port binding exists
  warning port_state       - Port is down
  error   chassis_health   - VIF port not bound to any chassis
  healthy type_consistency - LSP type "" is consistent with PortBinding type ""
  healthy address_config   - 1 address(es) configured
```

| | Command line | Northwatch |
|---|---|---|
| Steps | 7 commands (3 + one per chassis + logs) | 1 page |
| Hosts | every hypervisor | 1 browser tab |
| Scales with | number of compute nodes | nothing |
| Finds ports nobody reported yet | no | yes, all 42 ports are checked |

## Scenario 3: Can A reach B? {#scenario-3}

*Can `nw-ls-003-vif-004` (10.12.0.13) open a TCP connection to
`nw-ls-006-vif-001` (10.15.0.10)? They are on different switches behind the same
router.*

### With the command line

`ovn-trace` needs a complete packet description, and none of the values are
things a user tells you. Collect them first.

**1.** Source MAC and IP:

```console
$ nbctl --bare --columns=addresses list Logical_Switch_Port nw-ls-003-vif-004
02:00:00:30:03:04 10.12.0.13
```

**2.** Destination IP:

```console
$ nbctl --bare --columns=addresses list Logical_Switch_Port nw-ls-006-vif-001
02:00:00:30:06:01 10.15.0.10
```

**3.** The destination is routed, so `eth.dst` must be the MAC of the router
port that serves the source subnet, not the destination's MAC. (This is the
`N1SUBNET_MAC` step in the upstream tutorial.)

```console
$ nbctl --columns=name,mac,networks find Logical_Router_Port 'networks{>=}"10.12.0.1/24"'
name                : nw-lrp-003-003
mac                 : "02:00:00:10:03:03"
networks            : ["10.12.0.1/24"]
```

**4.** Assemble the expression and trace:

```console
$ docker exec clab-nw-lab-central ovn-trace --summary nw-ls-003 \
    'inport=="nw-ls-003-vif-004" && eth.src==02:00:00:30:03:04 && eth.dst==02:00:00:10:03:03 &&
     ip4.src==10.12.0.13 && ip4.dst==10.15.0.10 && ip.ttl==64 && tcp && tcp.dst==80'
ingress(dp="nw-ls-003", inport="nw-ls-003-vif-004") {
    ct_next(dnat);
    ct_next(ct_state=est|trk /* default (use --ct to customize) */) {
        ip.dscp = 32;
        outport = "nw-ls-003-lr";
        output;
        egress(dp="nw-ls-003", inport="nw-ls-003-vif-004", outport="nw-ls-003-lr") {
            output;
            /* output to "nw-ls-003-lr", type "patch" */;
            ingress(dp="nw-lr-003", inport="nw-lrp-003-003") {
                ip.ttl--;
                reg0 = ip4.dst;
                eth.src = 02:00:00:10:03:06;
                outport = "nw-lrp-003-006";
                eth.dst = 02:00:00:30:06:01;
                output;
                egress(dp="nw-lr-003", inport="nw-lrp-003-003", outport="nw-lrp-003-006") {
                    output;
                    /* output to "nw-lrp-003-006", type "patch" */;
                    ingress(dp="nw-ls-006", inport="nw-ls-006-lr") {
                        outport = "nw-ls-006-vif-001";
                        output;
                        egress(dp="nw-ls-006", inport="nw-ls-006-lr", outport="nw-ls-006-vif-001") {
                            ct_next(dnat);
                            ct_next(ct_state=est|trk /* default (use --ct to customize) */) {
                                output;
                                /* output to "nw-ls-006-vif-001", type "" */;
```

(Register assignments and `next;` lines removed for brevity.)

Get one digit of the router MAC wrong and the same command ends like this, with
no hint that the input was the problem:

```text
        outport = get_fdb(eth.dst);
        next;
        drop;
```

And a successful logical trace still says nothing about whether both ports are
actually bound and whether a tunnel exists between their hypervisors. That is
scenario 1, twice, plus an `Encap` lookup.

### With Northwatch

Open **Debug → Connectivity**, pick the two ports by name, and click **Check
Connectivity**. There is no match expression to write and no MAC to look up:

![Connectivity Checker result](/img/northwatch-vs-cli/connectivity.jpg)

The check covers the logical path (switch, router, ACLs) *and* the physical
realization (both bindings up, tunnel encaps between `chassis-3` and
`chassis-2`) in one pass.

```bash
curl -s 'http://localhost:8080/api/v1/debug/connectivity?src=719b170a-e232-49bb-abb4-eac51a530266&dst=bf7f092d-4a43-4f90-9e8d-c01438127c75'
```

::: tip Where ovn-trace still wins
The Connectivity Checker reasons about topology, ACLs and bindings; **Debug →
Packet Trace** shows the logical flows a port's traffic hits on its datapath.
Neither simulates a full microflow across datapaths the way `ovn-trace` does.
When you need register-level detail for one specific packet, `ovn-trace` is
still the tool. Northwatch gets you the verdict, and the inputs for that
trace, without the lookups.
:::

| | Command line | Northwatch |
|---|---|---|
| Steps | 3 lookups + 1 trace | pick 2 ports, 1 click |
| Values copied by hand | 5 (2 MACs, 2 IPs, router-port MAC) | 0 |
| Failure mode on a typo | silent `drop` | not possible |
| Physical realization checked | no, separate work | yes |

## Scenario 4: From an OpenFlow flow back to OVN {#scenario-4}

*You are on a hypervisor looking at `br-int`. Which OVN logical flow, port and
datapath does this OpenFlow rule belong to?* This is the centrepiece of the
upstream tutorial's physical-tracing section, and the most copy-paste-heavy
chain of all.

### With the command line

**1.** Find the OpenFlow port number of the interface (on the hypervisor):

```console
$ on chassis-3 ovs-vsctl --bare --columns=ofport find Interface external_ids:iface-id=nw-ls-003-vif-004
2
```

**2.** Feed **the ofport** and the MACs from scenario 3 into `ofproto/trace`:

```console
$ on chassis-3 ovs-appctl ofproto/trace br-int \
    in_port=2,tcp,dl_src=02:00:00:30:03:04,dl_dst=02:00:00:10:03:03,nw_src=10.12.0.13,nw_dst=10.15.0.10,nw_ttl=64,tp_dst=80
bridge("br-int")
----------------
 0. in_port=2, priority 100, cookie 0x7b2c1575
    set_field:0xa/0xffff->reg13
    set_field:0xf->reg11
    set_field:0x10->reg12
    set_field:0xa->metadata
    set_field:0x4->reg14
    set_field:0/0xffff0000->reg13
    resubmit(,8)
 8. metadata=0xa, priority 50, cookie 0xbf2ed567
    set_field:0/0x1000->reg10
    ...
```

The full output is 452 lines and references 98 distinct cookies. (The lab only
carries control-plane state, so ignore the final forwarding verdict; the
cookies are what matters here.)

**3.** Decode what you see. Nothing in the output is a name:

- `metadata=0xa` is the datapath's tunnel key (decimal 10). Look it up:

  ```console
  $ sbctl --columns=tunnel_key,external_ids find Datapath_Binding tunnel_key=10
  tunnel_key          : 10
  external_ids        : {logical-switch="98f08f5c-9bfb-40da-8473-2e88b001f0b5", name=nw-ls-003}
  ```

- `reg14=0x4` is the logical input port's tunnel key *within that datapath*
  (the `tunnel_key : 4` from scenario 1, step 2).
- Each `cookie` is the first 32 bits of the UUID of the Southbound row that
  produced the flow. Table 0's `0x7b2c1575` is the `Port_Binding` from
  scenario 1. For the others, copy **the cookie** back to the control plane:

  ```console
  $ sbctl lflow-list 0xbf2ed567
  Datapath: "nw-lr-001-pub" (c7a367c6-0d98-449c-b720-380b7b9a9f5a)  Pipeline: ingress
    table=0 (ls_in_check_port_sec), priority=50   , match=(1), action=(reg0[15] = check_in_port_sec(); next;)
  Datapath: "nw-lr-002-pub" (465b701c-878c-4b2e-9032-21a32a3cc679)  Pipeline: ingress
    table=0 (ls_in_check_port_sec), priority=50   , match=(1), action=(reg0[15] = check_in_port_sec(); next;)
  ...
  ```

Repeat step 3 for every table you care about. Upstream ships `ovn-detrace` to
automate the cookie lookups, but it needs the OVS Python bindings and database
access from the hypervisor, which is frequently not the case (it is not in this
lab, either).

### With Northwatch

Paste the cookie, without the `0x`, into the search field. A cookie is a UUID
prefix, and Omnisearch matches UUID prefixes across both databases, so
`bf2ed567` lands directly on the logical flow and `7b2c1575` on the port
binding:

![Logical flow found from an OpenFlow cookie](/img/northwatch-vs-cli/logical-flow.jpg)

From there **Visualize → Flow Pipeline** shows the flow in the context of its
datapath's ingress and egress tables, and the OVS interface page from
scenario 1 already gave you the ofport ↔ logical port ↔ datapath mapping with
names, so the `metadata`/`reg14` decoding in step 3 disappears entirely.

```bash
curl -s 'http://localhost:8080/api/v1/search?q=bf2ed567'
curl -s http://localhost:8080/api/v1/ovs/chassis-3/interface/4528f41a-3972-4f4a-968c-1f3ea877f782/correlation
```

| | Command line | Northwatch |
|---|---|---|
| Steps | 2 + 1–3 lookups *per table of interest* | 1 search per cookie |
| Hosts | hypervisor ↔ control plane, back and forth | 1 browser tab |
| Values copied by hand | ofport, MACs, every cookie, 2 tunnel keys | the cookie |
| Hex → decimal conversions | yes | none |

## Scenario 5: Which chassis is the active gateway? {#scenario-5}

*Router `nw-lr-002` has lost external connectivity. Which chassis should be
serving its gateway port, and which one is?*

### With the command line

**1.** Find the HA group behind the gateway port:

```console
$ nbctl --columns=name,ha_chassis_group find Logical_Router_Port name=nw-lr-002-gw
name                : nw-lr-002-gw
ha_chassis_group    : a47034b1-ce5a-4dbb-8752-670e35be6e44
```

**2.** Copy the **group UUID** to list the members, which are more UUIDs:

```console
$ nbctl --columns=name,ha_chassis list HA_Chassis_Group a47034b1-ce5a-4dbb-8752-670e35be6e44
name                : nw-hagrp-lr-002
ha_chassis          : [0b1125f9-5c2a-43e5-8d6e-8ab759511bab, 5a281f06-784c-4765-b4e0-6ecd5204e30e, 965c6be9-0e1e-47df-bbe3-5ba9eb59ed01]
```

**3–5.** Resolve each **member UUID** to get chassis and priority:

```console
$ nbctl --columns=chassis_name,priority list HA_Chassis 0b1125f9-5c2a-43e5-8d6e-8ab759511bab
chassis_name        : chassis-3
priority            : 80
$ nbctl --columns=chassis_name,priority list HA_Chassis 5a281f06-784c-4765-b4e0-6ecd5204e30e
chassis_name        : chassis-1
priority            : 100
$ nbctl --columns=chassis_name,priority list HA_Chassis 965c6be9-0e1e-47df-bbe3-5ba9eb59ed01
chassis_name        : chassis-2
priority            : 90
```

So `chassis-1` *should* be active. **6.** Who actually is? That lives in
Southbound, under a derived port name (`cr-` + the router port name):

```console
$ sbctl --columns=logical_port,chassis find Port_Binding type=chassisredirect
logical_port        : cr-nw-lr-003-gw
chassis             : c2b600e1-fea5-46ab-946c-74037861b31c

logical_port        : cr-nw-lr-001-gw
chassis             : c2b600e1-fea5-46ab-946c-74037861b31c

logical_port        : cr-nw-lr-002-gw
chassis             : []
```

**7.** `cr-nw-lr-002-gw` has no chassis at all. For the healthy ones you would
still have to resolve `c2b600e1-…` to a name with one more `sbctl list Chassis`.
And routers that use the older `gateway_chassis` column instead of an HA group
(`nw-lr-001` here) need a different first command altogether
(`lrp-get-gateway-chassis`).

### With Northwatch

Open **Visualize → HA Failover**. Desired and actual ownership are side by side
for every gateway, both configuration styles included, and the gateway nobody
owns is listed as an anomaly at the top:

![HA Failover view with a gateway that has no active chassis](/img/northwatch-vs-cli/ha-failover.jpg)

(The `no-owner` state is [expected in the userspace-datapath lab](/how-to/run-the-local-lab#expected-alerts):
BFD never converges there. It is precisely the kind of incomplete realization
you want pointed out.)

```bash
curl -s http://localhost:8080/api/v1/topology/gateway
```

| | Command line | Northwatch |
|---|---|---|
| Steps | 7 commands for one router | 1 page for all routers |
| Values copied by hand | 5 UUIDs | 0 |
| Desired vs. actual | compare two outputs yourself | shown together, mismatch flagged |

## Scenario 6: Which ACLs are in force? {#scenario-6}

*Which ACLs exist in this deployment, where are they attached, and do they
actually apply to anything?*

### With the command line

ACLs hang off logical switches **or** port groups, and `acl-list` takes exactly
one of them at a time. So: one command per switch…

```console
$ nbctl acl-list nw-ls-001
  to-lport  1000 (ip4.src == 10.10.0.0/24) allow-related
$ nbctl acl-list nw-ls-002
  to-lport  1000 (ip4.src == 10.11.0.0/24) allow-related
$ nbctl acl-list nw-ls-003
  to-lport  1000 (ip4.src == 10.12.0.0/24) allow-related
```

…for all nine switches, then discover the port groups and repeat per group:

```console
$ nbctl --columns=name,ports list Port_Group
name                : nw-pg-web
ports               : []
$ nbctl acl-list nw-pg-web
  to-lport  1100 (outport == @nw-pg-web && tcp.dst == 80) allow-related
  to-lport  1000 (outport == @nw-pg-web && ip) drop
```

To answer "which ACLs apply to port X" you would additionally search every port
group's `ports` column for X's UUID, and resolve any `$address_set` in the
matches with `nbctl list Address_Set`.

### With Northwatch

**Visualize → Security Policy** shows every port group with its ACLs and member
count, plus all standalone ACLs, on one page with a filter box:

![Security Policy view](/img/northwatch-vs-cli/security-policy.jpg)

Note the `0 ports` badge on `nw-pg-web`: there is a `drop` rule in this
deployment that matches nothing. On the command line that fact was there too
(`ports : []`), just in a different output than the rule it neutralizes.
**Debug → ACL Audit** goes further and reports shadowed and conflicting rules.

```bash
curl -s http://localhost:8080/api/v1/debug/acl-audit
```

| | Command line | Northwatch |
|---|---|---|
| Steps | 1 per switch + 1 per port group + lookups (12 here) | 1 page |
| Scales with | number of switches and port groups | nothing |
| Rule ↔ membership side by side | no | yes |

## Scenario 7: What NAT is configured? {#scenario-7}

*Which external IPs does this deployment use, and for which internal networks?*

### With the command line

`lr-nat-list` works per router, so list the routers, then ask each one:

```console
$ nbctl lr-nat-list nw-lr-001
TYPE             GATEWAY_PORT          MATCH                 EXTERNAL_IP        EXTERNAL_PORT    LOGICAL_IP          EXTERNAL_MAC         LOGICAL_PORT
snat                                                         192.0.2.1                           10.10.0.0/24
snat                                                         192.0.2.1                           10.13.0.0/24
$ nbctl lr-nat-list nw-lr-002
snat                                                         192.0.2.2                           10.11.0.0/24
snat                                                         192.0.2.2                           10.14.0.0/24
$ nbctl lr-nat-list nw-lr-003
snat                                                         192.0.2.3                           10.15.0.0/24
snat                                                         192.0.2.3                           10.12.0.0/24
```

Whether that SNAT can work also depends on the default route
(`nbctl lr-route-list <router>`, again per router) and on the gateway chassis
(all of scenario 5).

### With Northwatch

**Visualize → NAT Overview** groups NAT rules and static routes by router, with
totals and a filter that answers "who uses 192.0.2.2?" by typing it:

![NAT Overview grouped by router](/img/northwatch-vs-cli/nat-overview.jpg)

**Visualize → Load Balancers** does the same for VIPs and their backends.

```bash
curl -s http://localhost:8080/api/v1/topology/nat
```

| | Command line | Northwatch |
|---|---|---|
| Steps | 1 + 2 per router (7 here) | 1 page |
| Reverse lookup by external IP | `grep` across all outputs | filter box |

## Scenario 8: What depends on this router? {#scenario-8}

*Someone wants to delete `nw-lr-003`. What goes with it?*

### With the command line

There is no command for this. You walk the references by hand: `list
Logical_Router` gives you UUID lists for `ports`, `nat`, `policies` and
`static_routes`; each of those is a `list <Table> <uuid>` call; gateway ports
lead to `Gateway_Chassis` rows; and the Southbound side (`Datapath_Binding`,
the `Port_Binding` of every router port and its peer) has to be found through
`external_ids` matches. For this small router that is roughly 15 commands and
as many copied UUIDs — and you have to know the schema well enough to know
where to look.

### With Northwatch

Open the router (search for `nw-lr-003`, then **Raw**) and click **Impact
Analysis**. Northwatch walks the schema's strong and weak references and the
Northbound↔Southbound correlation for you:

![Impact analysis for a logical router](/img/northwatch-vs-cli/impact-analysis.jpg)

```bash
curl -s http://localhost:8080/api/v1/impact/nb/Logical_Router/8e281f3c-b767-4fd2-8bbb-021b9b0bebb0
```

```json
{
  "total_affected": 13,
  "by_table": {
    "Logical_Router_Port": 3, "NAT": 2, "Logical_Router_Policy": 1,
    "Logical_Router_Static_Route": 1, "Gateway_Chassis": 1,
    "Datapath_Binding": 1, "Port_Binding": 4
  },
  "by_ref_type": { "strong": 8, "correlation": 5 }
}
```

Impact analysis ships with the write API, so it is only available when
Northwatch runs with `--write-enabled`; see
[Enable write operations](/how-to/enable-write-operations).

| | Command line | Northwatch |
|---|---|---|
| Steps | ~15 commands, schema knowledge required | 1 click |
| Southbound objects included | only if you think of them | yes |

## The tally

| # | Question | Command line | Northwatch |
|---|---|---|---|
| 1 | Where does this IP live? | 5 commands, 2 hosts, 3 copied values | 1 search, 2 clicks |
| 2 | Why is this port down? | 7 commands, every hypervisor | 1 page |
| 3 | Can A reach B? | 4 commands, 5 copied values | pick 2 ports, 1 click |
| 4 | OpenFlow flow → OVN | 2 + up to 3 per table, 2 hosts, hex decoding | 1 search per cookie |
| 5 | Which chassis is the active gateway? | 7 commands, 5 copied UUIDs | 1 page |
| 6 | Which ACLs are in force? | 12 commands | 1 page |
| 7 | What NAT is configured? | 7 commands | 1 page |
| 8 | What depends on this router? | ~15 commands | 1 click |

Three things are behind those numbers:

- **OVSDB references are UUIDs, and the CLI does not follow them.** Most
  commands in the left column exist only to turn the previous output's UUID into
  a name. Northwatch holds both databases in memory and resolves references as
  it renders.
- **The answer spans hosts.** Northbound and Southbound live on the control
  plane, `Open_vSwitch` lives on each hypervisor. The CLI makes you the
  transport between them; with per-chassis OVS visibility, Northwatch already
  has all three.
- **The CLI answers what you ask.** It will not tell you that Southbound and
  OVS disagree, that a gateway has no owner, or that a `drop` rule applies to
  zero ports. Northwatch checks continuously and puts those findings on top.

None of this makes the command-line tools obsolete: `ovn-trace` and
`ofproto/trace` remain the reference for packet-level detail. Northwatch gets
you to the right port, chassis, flow and cookie in seconds, so that when you do
reach for them you already have every value they need.

## Next steps

- [Investigate with Omnisearch](/tutorials/investigate-with-omnisearch) follows
  the correlation chain from scenario 1 step by step over the API.
- [Diagnose port bindings](/how-to/diagnose-port-bindings),
  [Trace a packet path](/how-to/trace-a-packet-path) and
  [Audit ACLs](/how-to/audit-acls) cover the debug tools in depth.
- [Copy a view for analysis](/how-to/copy-a-view-for-analysis) takes a whole
  view to an AI assistant in one click, instead of the fragments this tutorial
  keeps pasting by hand.
