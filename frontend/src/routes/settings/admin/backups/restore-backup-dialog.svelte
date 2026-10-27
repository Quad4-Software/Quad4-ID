<script lang="ts">
	import FormInput from '#lib/components/form/form-input.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as Dialog from '#lib/components/ui/dialog/index.ts';
	import * as RadioGroup from '#lib/components/ui/radio-group/index.ts';
	import { m } from '#lib/paraglide/messages.js';
	import BackupService from '#lib/services/backup-service.ts';
	import type { Backup } from '#lib/types/backup.type.ts';
	import { axiosErrorToast } from '#lib/utils/error-util.ts';
	import { toast } from 'svelte-sonner';

	let {
		backup = $bindable(),
		open = $bindable(),
		onRestored
	}: {
		backup: Backup | null;
		open: boolean;
		onRestored: () => void;
	} = $props();

	const backupService = new BackupService();

	let source = $state<'existing' | 'upload'>('existing');
	let file = $state<File | null>(null);
	let encryptionKeyInput = $state({ value: '', error: null as string | null, required: false });
	let restoring = $state(false);

	$effect(() => {
		if (open) {
			source = 'existing';
			file = null;
			encryptionKeyInput.value = '';
		}
	});

	function onFileChange(e: Event) {
		const input = e.target as HTMLInputElement;
		file = input.files?.[0] ?? null;
	}

	async function restore() {
		restoring = true;
		try {
			if (source === 'upload') {
				if (!file) return;
				await backupService.restore({ file, encryptionKey: encryptionKeyInput.value });
			} else {
				if (!backup) return;
				await backupService.restore({ name: backup.name, encryptionKey: encryptionKeyInput.value });
			}
			toast.success(m.restore_staged_restart_required());
			open = false;
			onRestored();
		} catch (e) {
			axiosErrorToast(e);
		} finally {
			restoring = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>{m.restore_backup()}</Dialog.Title>
			<Dialog.Description>{m.restore_backup_description()}</Dialog.Description>
		</Dialog.Header>

		<div class="flex flex-col gap-5 py-2">
			<RadioGroup.Root bind:value={source}>
				<div class="flex items-center space-x-2">
					<RadioGroup.Item value="existing" id="restore-existing" />
					<label for="restore-existing" class="text-sm font-medium">
						{m.restore_from_existing()}
					</label>
				</div>
				<div class="flex items-center space-x-2">
					<RadioGroup.Item value="upload" id="restore-upload" />
					<label for="restore-upload" class="text-sm font-medium">
						{m.restore_from_upload()}
					</label>
				</div>
			</RadioGroup.Root>

			{#if source === 'existing'}
				<p class="text-muted-foreground font-mono text-xs break-all">
					{backup?.name}
				</p>
			{:else}
				<FormInput label={m.backup_file()} type="text" labelFor="restore-file">
					<input
						id="restore-file"
						type="file"
						accept=".zip,.enc"
						class="border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-9 w-full rounded-md border px-3 py-1 text-sm shadow-xs transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium focus-visible:ring-1 focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-50"
						onchange={onFileChange}
					/>
				</FormInput>
			{/if}

			<FormInput
				label={m.backup_encryption_key()}
				type="password"
				description={m.restore_encryption_key_description()}
				bind:input={encryptionKeyInput}
			/>

			<p class="text-destructive text-sm font-medium">
				{m.restore_warning()}
			</p>
		</div>

		<Dialog.Footer>
			<Button variant="outline" onclick={() => (open = false)}>{m.cancel()}</Button>
			<Button
				variant="destructive"
				onclick={restore}
				disabled={restoring || (source === 'upload' && !file) || (source === 'existing' && !backup)}
			>
				{restoring ? m.restoring() : m.stage_restore()}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
