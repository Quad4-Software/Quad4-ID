import type { UserGroup } from './user-group.type';

export interface SignupToken {
	id: string;
	token: string;
	expiresAt: string;
	usageLimit: number;
	usageCount: number;
	requireApproval: boolean;
	note: string;
	userGroups: UserGroup[];
	createdAt: string;
}

export interface InviteStatus {
	valid: boolean;
	requireApproval: boolean;
	expiresAt?: string;
}
