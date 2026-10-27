export type Backup = {
	name: string;
	size: number;
	createdAt: string;
	encrypted: boolean;
};

export type PendingRestore = {
	name: string;
	encrypted: boolean;
	size: number;
	stagedAt: string;
};

export type RestoreResult = {
	name: string;
	success: boolean;
	error?: string;
	finishedAt: string;
};

export type PendingRestoreResponse = {
	pending: PendingRestore | null;
	result: RestoreResult | null;
};
