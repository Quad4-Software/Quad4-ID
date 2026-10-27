<script lang="ts">
	import FormInput from '#lib/components/form/form-input.svelte';
	import SwitchWithLabel from '#lib/components/form/switch-with-label.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as Field from '#lib/components/ui/field/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import { m } from '#lib/paraglide/messages.js';
	import appConfigStore from '#lib/stores/application-configuration-store.ts';
	import type { AllAppConfig } from '#lib/types/application-configuration.type.ts';
	import { createForm, pickSchemaValues } from '#lib/utils/form-util.ts';
	import { trackFormChanges } from '#lib/utils/unsaved-changes-util.svelte.ts';
	import { z } from 'zod/v4';

	let {
		callback,
		appConfig
	}: {
		appConfig: AllAppConfig;
		callback: (appConfig: Partial<AllAppConfig>) => Promise<void>;
	} = $props();

	const formSchema = z.object({
		backupEnabled: z.boolean(),
		backupDestination: z.enum(['local', 's3']),
		backupLocalPath: z.string().min(1),
		backupIntervalHours: z.number().int().min(1).max(8760),
		backupRetentionCount: z.number().int().min(0),
		backupEncryptionKey: z.string(),
		backupS3Bucket: z.string(),
		backupS3Region: z.string(),
		backupS3Endpoint: z.string(),
		backupS3AccessKeyId: z.string(),
		backupS3SecretAccessKey: z.string(),
		backupS3Prefix: z.string(),
		backupS3ForcePathStyle: z.boolean()
	});

	let formStore = $derived(createForm(formSchema, pickSchemaValues(formSchema, appConfig)));
	let inputs = $derived(formStore.inputs);

	const destinationOptions = [
		{ label: m.backup_destination_local(), value: 'local' },
		{ label: m.backup_destination_s3(), value: 's3' }
	];

	function generateKey() {
		const bytes = new Uint8Array(24);
		crypto.getRandomValues(bytes);
		$inputs.backupEncryptionKey.value = btoa(String.fromCharCode(...bytes));
	}

	trackFormChanges(() => formStore, callback);
</script>

<fieldset class="flex flex-col gap-5" disabled={$appConfigStore.uiConfigDisabled}>
	<div class="flex flex-col gap-5">
		<SwitchWithLabel
			id="backup-enabled"
			label={m.backup_enable_scheduled()}
			description={m.backup_enable_scheduled_description()}
			bind:checked={$inputs.backupEnabled.value}
		/>
		<Field.Field>
			<Field.Label>{m.backup_destination()}</Field.Label>
			<Field.Description>{m.backup_destination_description()}</Field.Description>
			<Select.Root
				type="single"
				value={$inputs.backupDestination.value}
				onValueChange={(v) => ($inputs.backupDestination.value = v as 'local' | 's3')}
			>
				<Select.Trigger class="w-full" aria-label={m.backup_destination()}>
					{destinationOptions.find((o) => o.value === $inputs.backupDestination.value)?.label}
				</Select.Trigger>
				<Select.Content>
					{#each destinationOptions as option (option.value)}
						<Select.Item value={option.value}>{option.label}</Select.Item>
					{/each}
				</Select.Content>
			</Select.Root>
		</Field.Field>

		{#if $inputs.backupDestination.value === 'local'}
			<FormInput
				label={m.backup_local_path()}
				description={m.backup_local_path_description()}
				bind:input={$inputs.backupLocalPath}
			/>
		{:else}
			<div class="grid grid-cols-1 items-start gap-5 md:grid-cols-2">
				<FormInput label={m.backup_s3_bucket()} bind:input={$inputs.backupS3Bucket} />
				<FormInput label={m.backup_s3_region()} bind:input={$inputs.backupS3Region} />
				<FormInput
					label={m.backup_s3_endpoint()}
					description={m.backup_s3_endpoint_description()}
					bind:input={$inputs.backupS3Endpoint}
				/>
				<FormInput label={m.backup_s3_prefix()} bind:input={$inputs.backupS3Prefix} />
				<FormInput label={m.backup_s3_access_key_id()} bind:input={$inputs.backupS3AccessKeyId} />
				<FormInput
					label={m.backup_s3_secret_access_key()}
					type="password"
					bind:input={$inputs.backupS3SecretAccessKey}
				/>
			</div>
			<SwitchWithLabel
				id="backup-s3-force-path-style"
				label={m.backup_s3_force_path_style()}
				description={m.backup_s3_force_path_style_description()}
				bind:checked={$inputs.backupS3ForcePathStyle.value}
			/>
		{/if}

		<div class="grid grid-cols-1 items-start gap-5 md:grid-cols-2">
			<FormInput
				label={m.backup_interval_hours()}
				type="number"
				description={m.backup_interval_hours_description()}
				bind:input={$inputs.backupIntervalHours}
			/>
			<FormInput
				label={m.backup_retention_count()}
				type="number"
				description={m.backup_retention_count_description()}
				bind:input={$inputs.backupRetentionCount}
			/>
		</div>

		<FormInput
			label={m.backup_encryption_key()}
			type="password"
			description={m.backup_encryption_key_description()}
			bind:input={$inputs.backupEncryptionKey}
		/>
		<div>
			<Button variant="outline" size="sm" onclick={generateKey}>{m.backup_generate_key()}</Button>
		</div>
	</div>
</fieldset>
