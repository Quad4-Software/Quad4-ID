export interface SharedProps {
	logoURL: string;
	appName: string;
	appURL: string;
}

export const sharedPreviewProps: SharedProps = {
	logoURL: 'https://example.com/logo.png',
	appName: 'Quad4 ID',
	appURL: 'https://id.example.com'
};

export const sharedTemplateProps: SharedProps = {
	logoURL: '{{.LogoURL}}',
	appName: '{{.AppName}}',
	appURL: '{{.AppURL}}'
};
