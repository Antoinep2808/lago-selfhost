FROM getlago/api:v1.33.2

# Ports et environnement
EXPOSE 3000
ENV RAILS_ENV=production
ENV RACK_ENV=production
ENV LAGO_ENV=production

# Patch: définir WEB_CONCURRENCY à une valeur par défaut
ENV WEB_CONCURRENCY=1

# Lancer Puma
CMD ["bash", "-lc", "bundle exec puma -C config/puma.rb"]