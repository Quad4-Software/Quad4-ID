import type { Backup, PendingRestoreResponse } from '#lib/types/backup.type.ts';
import APIService from './api-service';

export default class BackupService extends APIService {
	list = async () => {
		const { data } = await this.api.get<Backup[]>('/backups');
		return data;
	};

	create = async () => {
		const { data } = await this.api.post<Backup>('/backups');
		return data;
	};

	remove = async (name: string) => {
		await this.api.delete(`/backups/${encodeURIComponent(name)}`);
	};

	download = async (name: string) => {
		const { data } = await this.api.get(`/backups/${encodeURIComponent(name)}/download`, {
			responseType: 'blob'
		});
		const url = URL.createObjectURL(data);
		const a = document.createElement('a');
		a.href = url;
		a.download = name;
		a.click();
		URL.revokeObjectURL(url);
	};

	pendingRestore = async () => {
		const { data } = await this.api.get<PendingRestoreResponse>('/backups/pending-restore');
		return data;
	};

	cancelPendingRestore = async () => {
		await this.api.delete('/backups/pending-restore');
	};

	restore = async (opts: { name?: string; file?: File; encryptionKey?: string }) => {
		const formData = new FormData();
		if (opts.file) formData.append('file', opts.file);
		if (opts.name) formData.append('name', opts.name);
		if (opts.encryptionKey) formData.append('encryptionKey', opts.encryptionKey);

		const { data } = await this.api.post('/backups/restore', formData);
		return data;
	};
}
