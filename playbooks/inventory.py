#!/usr/bin/env python
import libvirt
import json
import sys


def get_kvm_domains():
    conn = libvirt.open("qemu:///system")
    if conn is None:
        print("Failed to open connection to qemu:///system", file=sys.stderr)
        return {}

    domains = {}

    # Get list of all active domain IDs
    for domain_id in conn.listDomainsID():
        domain = conn.lookupByID(domain_id)
        domain_name = domain.name()

        domain_ips = []
        for iface in domain.interfaceAddresses(
            libvirt.VIR_DOMAIN_INTERFACE_ADDRESSES_SRC_LEASE
        ).values():
            for ip in iface["addrs"]:
                if ip["type"] == libvirt.VIR_IP_ADDR_TYPE_IPV4:
                    domain_ips.append(ip["addr"])

        # Add domain name, its IP address, and custom vars (like ansible_host and ansible_user) to the inventory
        if domain_ips:
            domains[domain_name] = {
                "hosts": domain_ips,
                "vars": {
                    "ansible_host": domain_ips[0],
                    "ansible_user": "evgnomon",  # Change 'evgnomon' to the appropriate user
                },
            }

    conn.close()

    return domains


def generate_dynamic_inventory():
    inventory = {}

    kvm_domains = get_kvm_domains()
    for domain, details in kvm_domains.items():
        # Add each domain as a group with its IP and vars
        inventory[domain] = {"hosts": details["hosts"], "vars": details["vars"]}

    return inventory


if __name__ == "__main__":
    print(json.dumps(generate_dynamic_inventory(), indent=2))
