<script lang="ts">
	import { openConfirmDialog } from '#lib/components/confirm-dialog/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as Table from '#lib/components/ui/table/index.ts';
	import { m } from '#lib/paraglide/messages.js';
	import BackupService from '#lib/services/backup-service.ts';
	import type { Backup } from '#lib/types/backup.type.ts';
	import { axiosErrorToast } from '#lib/utils/error-util.ts';
	import {
		LucideDownload,
		LucideLock,
		LucideRefreshCcw,
		LucideTrash2,
		LucideUndo2
	} from '@lucide/svelte';
	import { toast } from 'svelte-sonner';

	let {
		backups = $bindable(),
		onRestore
	}: {
		backups: Backup[];
		onRestore: (backup: Backup) => void;
	} = $props();

	const backupService = new BackupService();
	let creating = $state(false);

	function formatSize(size: number) {
		if (size < 1024) return `${size} B`;
		const units = ['KB', 'MB', 'GB', 'TB'];
		let value = size;
		let unit = -1;
		do {
			value /= 1024;
			unit++;
		} while (value >= 1024 && unit < units.length - 1);
		return `${value.toFixed(1)} ${units[unit]}`;
	}

	function formatDate(dateStr: string) {
		return new Date(dateStr).toLocaleString();
	}

	async function refresh() {
		backups = await backupService.list().catch((e) => {
			axiosErrorToast(e);
			return backups;
		});
	}

	async function createBackup() {
		creating = true;
		await backupService
			.create()
			.then(() => toast.success(m.backup_created_successfully()))
			.catch(axiosErrorToast)
			.finally(refresh);
		creating = false;
	}

	async function deleteBackup(backup: Backup) {
		openConfirmDialog({
			title: m.delete_backup(),
			message: m.delete_backup_confirm({ name: backup.name }),
			confirm: {
				label: m.delete(),
				destructive: true,
				action: async () => {
					await backupService
						.remove(backup.name)
						.then(() => toast.success(m.backup_deleted()))
						.catch(axiosErrorToast)
						.finally(refresh);
				}
			}
		});
	}
</script>

<div class="flex items-center justify-between">
	<h4 class="text-lg font-semibold">{m.backup_archives()}</h4>
	<div class="flex gap-2">
		<Button variant="outline" size="sm" onclick={refresh}>
			<LucideRefreshCcw class="mr-1 size-4" />
			{m.refresh()}
		</Button>
		<Button size="sm" onclick={createBackup} disabled={creating}>
			{creating ? m.backup_creating() : m.backup_now()}
		</Button>
	</div>
</div>

{#if backups.length === 0}
	<p class="text-muted-foreground text-sm">{m.no_backups_yet()}</p>
{:else}
	<Table.Root>
		<Table.Header>
			<Table.Row>
				<Table.Head>{m.name()}</Table.Head>
				<Table.Head>{m.size()}</Table.Head>
				<Table.Head>{m.created()}</Table.Head>
				<Table.Head class="w-0"></Table.Head>
			</Table.Row>
		</Table.Header>
		<Table.Body>
			{#each backups as backup (backup.name)}
				<Table.Row>
					<Table.Cell class="font-mono text-xs">
						<span class="inline-flex items-center gap-1.5">
							{backup.name}
							{#if backup.encrypted}
								<LucideLock class="text-muted-foreground size-3.5" />
							{/if}
						</span>
					</Table.Cell>
					<Table.Cell>{formatSize(backup.size)}</Table.Cell>
					<Table.Cell>{formatDate(backup.createdAt)}</Table.Cell>
					<Table.Cell>
						<div class="flex justify-end gap-1">
							<Button
								variant="ghost"
								size="icon"
								title={m.download()}
								onclick={() => backupService.download(backup.name).catch(axiosErrorToast)}
							>
								<LucideDownload class="size-4" />
							</Button>
							<Button
								variant="ghost"
								size="icon"
								title={m.restore()}
								onclick={() => onRestore(backup)}
							>
								<LucideUndo2 class="size-4" />
							</Button>
							<Button
								variant="ghost"
								size="icon"
								title={m.delete()}
								onclick={() => deleteBackup(backup)}
							>
								<LucideTrash2 class="size-4" />
							</Button>
						</div>
					</Table.Cell>
				</Table.Row>
			{/each}
		</Table.Body>
	</Table.Root>
{/if}
