FROM getlago/api:v1.33.2

EXPOSE 3000

ENV RAILS_ENV=production
ENV RACK_ENV=production
ENV LAGO_ENV=production
ENV WEB_CONCURRENCY=1

# Patch: retire la dépendance à `.present?` dans Puma
RUN sed -i "s/if ENV\\['WEB_CONCURRENCY'\\].present?/if ENV['WEB_CONCURRENCY']/" /app/config/puma.rb

# Démarrage du serveur Rails
CMD ["bash", "-lc", "bundle exec puma -C config/puma.rb"]