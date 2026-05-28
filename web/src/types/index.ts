export type Role = 'admin' | 'user';

export type User = {
  id: number;
  username: string;
  role: Role;
  has_api_key: boolean;
  temporary_mailbox_ttl_minutes: number[];
  can_lease_permanent_mailbox: boolean;
  openai_qos_rpm: number;
  created_at: string;
  updated_at: string;
};

export type Domain = {
  id: number;
  domain: string;
  owner_user_id: number | null;
  owner_name: string;
  disabled: boolean;
  created_at: string;
  updated_at: string;
};

export type MailMessage = {
  id: number;
  helo_name: string;
  mail_from: string;
  rcpt_to: string[];
  remote_addr: string;
  created_at: string;
};

export type TemporaryMailbox = {
  id: number;
  address: string;
  local_part: string;
  domain: string;
  owner_user_id: number;
  expires_at: string;
  is_permanent: boolean;
  created_at: string;
  expired: boolean;
};

export type MessageBody = {
  id: number;
  data: string;
  body: string;
  html: string;
  is_html: boolean;
};

export type ApiKeyResult = {
  token: string;
  user: User;
};

export type TemporaryMailboxCreateResult = TemporaryMailbox & {
  ttl_minutes: number;
};

export type PublicConfig = {
  smtp_hostname: string;
};

export type UserFormValues = {
  username: string;
  password: string;
  role: Role;
  temporary_mailbox_ttl_minutes: number[];
  can_lease_permanent_mailbox: boolean;
  openai_qos_rpm: number;
};

export type TemporaryMailboxFormValues = {
  domain: string;
  ttl_minutes?: number;
};

export type DomainFormValues = {
  domain: string;
  owner_user_id?: number | null;
  disabled?: boolean;
  verification_name?: string;
  verification_value?: string;
};

export type DomainVerification = {
  name: string;
  value: string;
};

export type ApiError = {
  error?: {
    code?: string;
    message?: string;
  };
};
