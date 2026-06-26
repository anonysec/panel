//go:build !lite

package api

// DefaultContent provides pre-built i18n landing page content for all supported
// languages. The content uses generic "business solutions" / portfolio copy that
// does not reveal the server's management purpose. No navigation links point to
// /portal/ or /dashboard/.
var DefaultContent = map[string]LandingContent{
	"en": {
		HeroTitle:    "Elevate Your Digital Presence",
		HeroSubtitle: "We help businesses grow through innovative technology solutions, strategic consulting, and world-class digital services.",
		Features: []Feature{
			{
				Title:       "Cloud Infrastructure",
				Description: "Scalable and reliable hosting solutions tailored to your business needs with 99.9% uptime guarantee.",
			},
			{
				Title:       "Digital Strategy",
				Description: "Expert consulting to help you navigate digital transformation and achieve measurable results.",
			},
			{
				Title:       "Web Development",
				Description: "Custom web applications built with modern frameworks, optimized for performance and accessibility.",
			},
			{
				Title:       "Data Analytics",
				Description: "Turn raw data into actionable insights with our advanced analytics and reporting platform.",
			},
		},
		Pricing: []PricingTier{
			{
				Name:  "Starter",
				Price: "$29/mo",
				Features: []string{
					"5 projects",
					"10 GB storage",
					"Email support",
					"Basic analytics",
				},
			},
			{
				Name:  "Professional",
				Price: "$79/mo",
				Features: []string{
					"Unlimited projects",
					"100 GB storage",
					"Priority support",
					"Advanced analytics",
					"Custom integrations",
				},
			},
			{
				Name:  "Enterprise",
				Price: "Contact us",
				Features: []string{
					"Dedicated resources",
					"Unlimited storage",
					"24/7 phone support",
					"Custom SLA",
					"On-premise deployment",
				},
			},
		},
		FAQ: []FAQItem{
			{
				Question: "How do I get started?",
				Answer:   "Simply choose a plan that fits your needs, create an account, and our onboarding team will guide you through setup.",
			},
			{
				Question: "Can I upgrade my plan later?",
				Answer:   "Yes, you can upgrade or downgrade your plan at any time. Changes take effect at the start of your next billing cycle.",
			},
			{
				Question: "Do you offer custom solutions?",
				Answer:   "Absolutely. Our Enterprise plan includes tailored solutions designed specifically for your organization's requirements.",
			},
			{
				Question: "What payment methods do you accept?",
				Answer:   "We accept all major credit cards, bank transfers, and digital payment methods.",
			},
		},
		FooterText:         "© 2024 All rights reserved. Built with care for businesses worldwide.",
		ShowPanelLink:      false,
		PanelLinkPlacement: "hidden",
	},
	"fa": {
		HeroTitle:    "حضور دیجیتال خود را ارتقا دهید",
		HeroSubtitle: "ما به کسب‌وکارها کمک می‌کنیم تا از طریق راه‌حل‌های فناوری نوآورانه، مشاوره استراتژیک و خدمات دیجیتال در سطح جهانی رشد کنند.",
		Features: []Feature{
			{
				Title:       "زیرساخت ابری",
				Description: "راه‌حل‌های میزبانی مقیاس‌پذیر و قابل اعتماد متناسب با نیازهای کسب‌وکار شما با تضمین ۹۹.۹٪ آپتایم.",
			},
			{
				Title:       "استراتژی دیجیتال",
				Description: "مشاوره تخصصی برای کمک به شما در مسیر تحول دیجیتال و دستیابی به نتایج قابل اندازه‌گیری.",
			},
			{
				Title:       "توسعه وب",
				Description: "برنامه‌های وب سفارشی با فریمورک‌های مدرن، بهینه‌سازی شده برای عملکرد و دسترسی‌پذیری.",
			},
			{
				Title:       "تحلیل داده",
				Description: "داده‌های خام را با پلتفرم تحلیل و گزارش‌دهی پیشرفته ما به بینش‌های عملی تبدیل کنید.",
			},
		},
		Pricing: []PricingTier{
			{
				Name:  "شروع",
				Price: "۲۹ دلار/ماه",
				Features: []string{
					"۵ پروژه",
					"۱۰ گیگابایت فضا",
					"پشتیبانی ایمیلی",
					"تحلیل پایه",
				},
			},
			{
				Name:  "حرفه‌ای",
				Price: "۷۹ دلار/ماه",
				Features: []string{
					"پروژه نامحدود",
					"۱۰۰ گیگابایت فضا",
					"پشتیبانی اولویت‌دار",
					"تحلیل پیشرفته",
					"یکپارچه‌سازی سفارشی",
				},
			},
			{
				Name:  "سازمانی",
				Price: "تماس بگیرید",
				Features: []string{
					"منابع اختصاصی",
					"فضای نامحدود",
					"پشتیبانی ۲۴/۷ تلفنی",
					"SLA سفارشی",
					"استقرار در محل",
				},
			},
		},
		FAQ: []FAQItem{
			{
				Question: "چگونه شروع کنم؟",
				Answer:   "کافی است طرحی متناسب با نیازهای خود انتخاب کنید، حساب کاربری بسازید و تیم ما شما را در راه‌اندازی راهنمایی خواهد کرد.",
			},
			{
				Question: "آیا می‌توانم بعداً طرح خود را ارتقا دهم؟",
				Answer:   "بله، شما می‌توانید هر زمان طرح خود را ارتقا یا تنزل دهید. تغییرات از ابتدای دوره صورت‌حساب بعدی اعمال می‌شود.",
			},
			{
				Question: "آیا راه‌حل‌های سفارشی ارائه می‌دهید؟",
				Answer:   "قطعاً. طرح سازمانی ما شامل راه‌حل‌های اختصاصی طراحی شده ویژه نیازهای سازمان شماست.",
			},
			{
				Question: "چه روش‌های پرداختی را قبول می‌کنید؟",
				Answer:   "ما تمام کارت‌های اعتباری اصلی، حواله بانکی و روش‌های پرداخت دیجیتال را می‌پذیریم.",
			},
		},
		FooterText:         "© ۲۰۲۴ تمامی حقوق محفوظ است. با عشق برای کسب‌وکارهای سراسر جهان ساخته شده.",
		ShowPanelLink:      false,
		PanelLinkPlacement: "hidden",
	},
	"zh": {
		HeroTitle:    "提升您的数字化形象",
		HeroSubtitle: "我们通过创新技术解决方案、战略咨询和世界一流的数字服务，帮助企业实现增长。",
		Features: []Feature{
			{
				Title:       "云基础设施",
				Description: "根据您的业务需求量身定制的可扩展、可靠的托管解决方案，保证99.9%的正常运行时间。",
			},
			{
				Title:       "数字战略",
				Description: "专业咨询帮助您驾驭数字化转型，实现可衡量的成果。",
			},
			{
				Title:       "网站开发",
				Description: "使用现代框架构建的定制网络应用程序，针对性能和可访问性进行了优化。",
			},
			{
				Title:       "数据分析",
				Description: "通过我们先进的分析和报告平台，将原始数据转化为可操作的洞察。",
			},
		},
		Pricing: []PricingTier{
			{
				Name:  "入门版",
				Price: "¥199/月",
				Features: []string{
					"5个项目",
					"10 GB存储空间",
					"邮件支持",
					"基础分析",
				},
			},
			{
				Name:  "专业版",
				Price: "¥549/月",
				Features: []string{
					"无限项目",
					"100 GB存储空间",
					"优先支持",
					"高级分析",
					"自定义集成",
				},
			},
			{
				Name:  "企业版",
				Price: "联系我们",
				Features: []string{
					"专属资源",
					"无限存储空间",
					"7×24小时电话支持",
					"定制SLA",
					"本地部署",
				},
			},
		},
		FAQ: []FAQItem{
			{
				Question: "如何开始使用？",
				Answer:   "只需选择适合您需求的方案，创建账户，我们的团队将引导您完成设置。",
			},
			{
				Question: "以后可以升级方案吗？",
				Answer:   "可以，您可以随时升级或降级方案。更改将在下一个计费周期开始时生效。",
			},
			{
				Question: "你们提供定制解决方案吗？",
				Answer:   "当然。我们的企业版包含专门为您的组织需求设计的定制解决方案。",
			},
			{
				Question: "接受哪些付款方式？",
				Answer:   "我们接受所有主要信用卡、银行转账和数字支付方式。",
			},
		},
		FooterText:         "© 2024 版权所有。为全球企业精心打造。",
		ShowPanelLink:      false,
		PanelLinkPlacement: "hidden",
	},
	"ru": {
		HeroTitle:    "Улучшите ваше цифровое присутствие",
		HeroSubtitle: "Мы помогаем бизнесу расти благодаря инновационным технологическим решениям, стратегическому консалтингу и цифровым услугам мирового класса.",
		Features: []Feature{
			{
				Title:       "Облачная инфраструктура",
				Description: "Масштабируемые и надёжные решения для хостинга, адаптированные под потребности вашего бизнеса с гарантией 99,9% времени работы.",
			},
			{
				Title:       "Цифровая стратегия",
				Description: "Экспертный консалтинг для навигации в цифровой трансформации и достижения измеримых результатов.",
			},
			{
				Title:       "Веб-разработка",
				Description: "Индивидуальные веб-приложения на современных фреймворках, оптимизированные для производительности и доступности.",
			},
			{
				Title:       "Аналитика данных",
				Description: "Превращайте необработанные данные в практические выводы с помощью нашей продвинутой аналитической платформы.",
			},
		},
		Pricing: []PricingTier{
			{
				Name:  "Стартовый",
				Price: "$29/мес",
				Features: []string{
					"5 проектов",
					"10 ГБ хранилища",
					"Поддержка по email",
					"Базовая аналитика",
				},
			},
			{
				Name:  "Профессиональный",
				Price: "$79/мес",
				Features: []string{
					"Неограниченное число проектов",
					"100 ГБ хранилища",
					"Приоритетная поддержка",
					"Расширенная аналитика",
					"Пользовательские интеграции",
				},
			},
			{
				Name:  "Корпоративный",
				Price: "Свяжитесь с нами",
				Features: []string{
					"Выделенные ресурсы",
					"Неограниченное хранилище",
					"Круглосуточная поддержка по телефону",
					"Индивидуальный SLA",
					"Локальное развёртывание",
				},
			},
		},
		FAQ: []FAQItem{
			{
				Question: "Как начать работу?",
				Answer:   "Просто выберите подходящий тарифный план, создайте аккаунт, и наша команда проведёт вас через процесс настройки.",
			},
			{
				Question: "Можно ли сменить тариф позже?",
				Answer:   "Да, вы можете повысить или понизить тариф в любое время. Изменения вступают в силу с начала следующего расчётного периода.",
			},
			{
				Question: "Вы предлагаете индивидуальные решения?",
				Answer:   "Безусловно. Корпоративный план включает решения, разработанные специально для потребностей вашей организации.",
			},
			{
				Question: "Какие способы оплаты вы принимаете?",
				Answer:   "Мы принимаем все основные кредитные карты, банковские переводы и цифровые платёжные системы.",
			},
		},
		FooterText:         "© 2024 Все права защищены. Создано с заботой для бизнеса по всему миру.",
		ShowPanelLink:      false,
		PanelLinkPlacement: "hidden",
	},
}
