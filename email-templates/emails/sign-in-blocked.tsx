import { BaseTemplate } from '../components/base-template';
import CardHeader from '../components/card-header';
import { DetailsList } from '../components/details-list';
import { Muted, Paragraph } from '../components/text';
import { type SharedProps, sharedPreviewProps, sharedTemplateProps } from '../props';

interface SignInBlockedData {
	location: string;
	ipAddress: string;
	device: string;
	method: string;
	dateTime: string;
}

interface SignInBlockedEmailProps extends SharedProps {
	data: SignInBlockedData;
}

export const SignInBlockedEmail = ({ data, ...props }: SignInBlockedEmailProps) => (
	<BaseTemplate
		{...props}
		preview={`A sign-in to your ${props.appName} account was blocked as unusual activity`}
	>
		<CardHeader title="Sign-in blocked" />
		<Paragraph>
			A sign-in to your {props.appName} account was blocked because it looked unusual. If this was
			you, sign in again from a network you have used before or contact your administrator.
		</Paragraph>

		<DetailsList
			items={[
				{ label: 'Approximate location', value: data.location },
				{ label: 'IP address', value: data.ipAddress },
				{ label: 'Device', value: data.device },
				{ label: 'Sign-in method', value: data.method },
				{ label: 'Time', value: data.dateTime }
			]}
		/>

		<Muted>
			If this was not you, review the passkeys in your {props.appName} account settings and remove
			any you don't recognize.
		</Muted>
	</BaseTemplate>
);

export default SignInBlockedEmail;

SignInBlockedEmail.TemplateProps = {
	...sharedTemplateProps,
	data: {
		location:
			'{{if and .Data.City .Data.Country}}{{.Data.City}}, {{.Data.Country}}{{else if .Data.Country}}{{.Data.Country}}{{else}}Unknown{{end}}',
		ipAddress: '{{.Data.IPAddress}}',
		device: '{{.Data.Device}}',
		method: '{{.Data.Method}}',
		dateTime: '{{.Data.DateTime.Format "January 2, 2006 at 3:04 PM MST"}}'
	}
};

SignInBlockedEmail.PreviewProps = {
	...sharedPreviewProps,
	data: {
		location: 'San Francisco, USA',
		ipAddress: '203.0.113.42',
		device: 'Chrome on macOS',
		method: 'Passkey',
		dateTime: 'January 2, 2026 at 3:04 PM UTC'
	}
};
