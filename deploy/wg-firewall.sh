#!/bin/sh
# Locks down the wg-easy network namespace so a WireGuard peer can reach
# exactly one thing: HTTPS on the server's VPN IP (Caddy -> chat app).
#
#  - No peer-to-peer traffic and no routing out to the LAN or internet, even
#    if a user edits their config to AllowedIPs = 0.0.0.0/0.
#  - The wg-easy admin UI, the app's loopback port, and anything else in the
#    namespace are unreachable over wg0.
#
# Runs as a sidecar sharing wg-easy's network namespace, and re-checks every
# 30s so the rules come back if wg-easy restarts its interface.
set -eu

WG_IF="${WG_IF:-wg0}"
WG_SERVER_IP="${WG_SERVER_IP:-10.8.0.1}"

apply() {
	iptables -N CHATWG_IN 2>/dev/null || true
	iptables -F CHATWG_IN
	iptables -A CHATWG_IN -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
	iptables -A CHATWG_IN -d "$WG_SERVER_IP" -p tcp --dport 443 -j ACCEPT
	iptables -A CHATWG_IN -d "$WG_SERVER_IP" -p icmp --icmp-type echo-request -j ACCEPT
	iptables -A CHATWG_IN -j DROP

	iptables -C INPUT -i "$WG_IF" -j CHATWG_IN 2>/dev/null ||
		iptables -I INPUT 1 -i "$WG_IF" -j CHATWG_IN
	iptables -C FORWARD -i "$WG_IF" -j DROP 2>/dev/null ||
		iptables -I FORWARD 1 -i "$WG_IF" -j DROP
	iptables -C FORWARD -o "$WG_IF" -j DROP 2>/dev/null ||
		iptables -I FORWARD 1 -o "$WG_IF" -j DROP

	# IPv6 is disabled for wg-easy, but drop anything on wg0 just in case.
	if command -v ip6tables >/dev/null 2>&1; then
		ip6tables -C INPUT -i "$WG_IF" -j DROP 2>/dev/null ||
			ip6tables -I INPUT 1 -i "$WG_IF" -j DROP 2>/dev/null || true
		ip6tables -C FORWARD -i "$WG_IF" -j DROP 2>/dev/null ||
			ip6tables -I FORWARD 1 -i "$WG_IF" -j DROP 2>/dev/null || true
	fi
}

apply
echo "wg-firewall: $WG_IF limited to tcp/443 on $WG_SERVER_IP"
touch /tmp/ready

while sleep 30; do
	# Keep our jumps first in the chains; re-insert if anything moved them.
	if [ "$(iptables -S INPUT | sed -n 2p)" != "-A INPUT -i $WG_IF -j CHATWG_IN" ] ||
		! iptables -C FORWARD -i "$WG_IF" -j DROP 2>/dev/null; then
		iptables -D INPUT -i "$WG_IF" -j CHATWG_IN 2>/dev/null || true
		apply
		echo "wg-firewall: rules re-applied"
	fi
done
