<script lang="ts">
	import * as Alert from '#lib/components/ui/alert/index.ts';
	import * as Card from '#lib/components/ui/card/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { m } from '#lib/paraglide/messages.js';
	import AppConfigService from '#lib/services/app-config-service.ts';
	import BackupService from '#lib/services/backup-service.ts';
	import appConfigStore from '#lib/stores/application-configuration-store.ts';
	import type { AllAppConfig } from '#lib/types/application-configuration.type.ts';
	import type { Backup, PendingRestoreResponse } from '#lib/types/backup.type.ts';
	import { axiosErrorToast } from '#lib/utils/error-util.ts';
	import { LucideInfo, LucideTriangleAlert } from '@lucide/svelte';
	import { toast } from 'svelte-sonner';
	import BackupForm from './backup-form.svelte';
	import BackupList from './backup-list.svelte';
	import RestoreBackupDialog from './restore-backup-dialog.svelte';

	let { data } = $props();
	let appConfig = $state(data.appConfig);
	let persistedAppConfig = { ...data.appConfig };
	let backups = $state<Backup[]>(data.backups);
	let pendingRestore = $state<PendingRestoreResponse | null>(data.pendingRestore);
	let backupToRestore: Backup | null = $state(null);
	let restoreDialogOpen = $state(false);

	const appConfigService = new AppConfigService();
	const backupService = new BackupService();

	async function updateAppConfig(updatedAppConfig: Partial<AllAppConfig>) {
		persistedAppConfig = await appConfigService.update({
			...persistedAppConfig,
			...updatedAppConfig
		});

		appConfigStore.set({ ...$appConfigStore, ...persistedAppConfig });
	}

	async function refreshPendingRestore() {
		pendingRestore = await backupService.pendingRestore().catch(() => pendingRestore);
	}

	async function cancelPendingRestore() {
		await backupService
			.cancelPendingRestore()
			.then(() => toast.success(m.pending_restore_canceled()))
			.catch(axiosErrorToast)
			.finally(refreshPendingRestore);
	}

	function onRestore(backup: Backup) {
		backupToRestore = backup;
		restoreDialogOpen = true;
	}
</script>

<svelte:head>
	<title>{m.backups()}</title>
</svelte:head>

{#if $appConfigStore.uiConfigDisabled}
	<Alert.Root variant="info">
		<LucideInfo class="size-4" />
		<Alert.Title>{m.ui_config_disabled_info_title()}</Alert.Title>
		<Alert.Description>
			{m.ui_config_disabled_info_description()}
		</Alert.Description>
	</Alert.Root>
{/if}

{#if pendingRestore?.pending}
	<Alert.Root variant="warning">
		<LucideTriangleAlert class="size-4" />
		<Alert.Title>{m.pending_restore_title()}</Alert.Title>
		<Alert.Description class="flex items-center justify-between gap-4">
			<span>{m.pending_restore_description({ name: pendingRestore.pending.name })}</span>
			<Button variant="outline" size="sm" onclick={cancelPendingRestore}>
				{m.cancel()}
			</Button>
		</Alert.Description>
	</Alert.Root>
{/if}

{#if pendingRestore?.result}
	<Alert.Root variant={pendingRestore.result.success ? 'info' : 'warning'}>
		<LucideInfo class="size-4" />
		<Alert.Title>
			{pendingRestore.result.success ? m.last_restore_succeeded() : m.last_restore_failed()}
		</Alert.Title>
		<Alert.Description>
			{#if pendingRestore.result.success}
				{m.last_restore_succeeded_description({ name: pendingRestore.result.name })}
			{:else}
				{pendingRestore.result.error}
			{/if}
		</Alert.Description>
	</Alert.Root>
{/if}

<Card.Root>
	<Card.Header>
		<Card.Title>{m.backup_configuration()}</Card.Title>
		<Card.Description>{m.backup_configuration_description()}</Card.Description>
	</Card.Header>
	<Card.Content>
		<BackupForm {appConfig} callback={updateAppConfig} />
	</Card.Content>
</Card.Root>

<Card.Root>
	<Card.Header>
		<Card.Title>{m.backups()}</Card.Title>
		<Card.Description>{m.backups_list_description()}</Card.Description>
	</Card.Header>
	<Card.Content>
		<BackupList bind:backups {onRestore} />
	</Card.Content>
</Card.Root>

<RestoreBackupDialog
	bind:backup={backupToRestore}
	bind:open={restoreDialogOpen}
	onRestored={refreshPendingRestore}
/>
