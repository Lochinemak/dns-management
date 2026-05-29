export type User = {
  id: string;
  email: string;
  nickname: string;
  role: 'user' | 'admin';
  createdAt?: string;
};

export type Domain = {
  id: string;
  name: string;
  zoneId: string;
  enabled: boolean;
  tokenMasked: string;
  createdAt: string;
  updatedAt: string;
};

export type Subdomain = {
  id: string;
  ownerId: string;
  ownerEmail?: string;
  domainId: string;
  domainName: string;
  prefix: string;
  fullDomain: string;
  status: 'pending' | 'active' | 'rejected' | 'suspended';
  rejectReason?: string;
  reviewedBy?: string;
  reviewedAt?: string;
  createdAt: string;
};

export type ReservedSubdomain = {
  id: string;
  domainId: string;
  domainName: string;
  prefix: string;
  fullDomain: string;
  createdBy: string;
  createdAt: string;
};

export type DnsRecord = {
  id: string;
  subdomainId: string;
  type: 'A' | 'AAAA' | 'CNAME' | 'TXT' | 'MX' | 'NS';
  name: string;
  content: string;
  ttl: number;
  proxied: boolean;
  createdAt?: string;
};

export type ApiToken = {
  id: string;
  userId: string;
  name: string;
  createdAt: string;
};

export type DnsRecordType = DnsRecord['type'];
