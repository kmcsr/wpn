
package main

const defaultBlockRuleContent = `#
# blocked IP rules
#

# localhost
127.0.0.1
::1

# docker
172.17.0.0/16

# other private networks
10.0.0.0/8
172.16.0.0/12
192.168.0.0/16
fd00::/8
`
