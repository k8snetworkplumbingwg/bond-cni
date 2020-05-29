## LACP bonding mode - experimental

Three parameters are used in the bonding config to configure LACP mode:

First mode must be set to "802.3ad".

Second LACP Rate can be set to either "fast" or "slow"

Third xmitHashPolicy must be set. This defaults to layer2 and can be set to any of:
* "layer2"   
* "layer3+4"
* "layer2+3"
* "encap2+3"
* "encap3+4"

Which of those is preferred depends your specific network design.
