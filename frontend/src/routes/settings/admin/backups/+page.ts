import AppConfigService from '#lib/services/app-config-service.ts';
import BackupService from '#lib/services/backup-service.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => {
	const appConfigService = new AppConfigService();
	const backupService = new BackupService();

	const appConfig = await appConfigService.list(true);
	const backups = await backupService.list().catch(() => []);
	const pendingRestore = await backupService.pendingRestore().catch(() => null);

	return { appConfig, backups, pendingRestore };
};
